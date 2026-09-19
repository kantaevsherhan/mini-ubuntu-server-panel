package httpapi

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	dockermanager "github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/docker"
)

func (a API) dockerUnavailable(c *fiber.Ctx) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "docker_service_unavailable"})
}

func dockerError(c *fiber.Ctx, err error, fallback string) error {
	switch {
	case errors.Is(err, dockermanager.ErrInvalid):
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "docker_request_invalid", "message": err.Error()})
	case errors.Is(err, dockermanager.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "docker_not_found", "message": err.Error()})
	case errors.Is(err, dockermanager.ErrConflict):
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "docker_conflict", "message": err.Error()})
	default:
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": fallback})
	}
}

func (a API) dockerAudit(c *fiber.Ctx, action, targetType, target, details string) {
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, action, targetType, target, details, c.IP())
}

func dockerList[T any](a API, c *fiber.Ctx, list func(context.Context) ([]T, error)) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	items, err := list(ctx)
	if err != nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "docker_unavailable"})
	}
	return c.JSON(items)
}

func (a API) dockerContainers(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	return dockerList(a, c, a.Docker.List)
}

func (a API) dockerContainerAction(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	id := strings.ToLower(strings.TrimSpace(c.Params("id")))
	var request struct {
		Action string `json:"action"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.Action = strings.ToLower(strings.TrimSpace(request.Action))
	if err := dockermanager.ValidateAction(id, request.Action); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "docker_action_invalid"})
	}
	if request.Action == "force-remove" && c.Locals("claims").(*auth.Claims).Role != "admin" {
		return fiber.ErrForbidden
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 25*time.Second)
	defer cancel()
	if err := a.Docker.Action(ctx, id, request.Action); err != nil {
		return dockerError(c, err, "docker_action_failed")
	}
	a.dockerAudit(c, "docker.container."+request.Action, "docker_container", id, `{"action":`+strconv.Quote(request.Action)+`}`)
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) dockerContainerLogs(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	id := strings.ToLower(strings.TrimSpace(c.Params("id")))
	tail := c.QueryInt("tail", 300)
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	output, err := a.Docker.Logs(ctx, id, tail)
	if err != nil {
		return dockerError(c, err, "docker_logs_failed")
	}
	return c.JSON(fiber.Map{"logs": output})
}

func (a API) dockerRunContainer(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	var request dockermanager.RunRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	request.Image = strings.TrimSpace(request.Image)
	request.Name = strings.TrimSpace(request.Name)
	if err := dockermanager.ValidateRunRequest(request); err != nil {
		return dockerError(c, err, "docker_run_failed")
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 25*time.Second)
	defer cancel()
	id, err := a.Docker.Run(ctx, request)
	if id != "" {
		a.dockerAudit(c, "docker.container.create", "docker_container", id, `{"image":`+strconv.Quote(request.Image)+`,"name":`+strconv.Quote(request.Name)+`,"ports":`+strconv.Itoa(len(request.Ports))+`,"volumes":`+strconv.Itoa(len(request.Volumes))+`}`)
	}
	if err != nil {
		return dockerError(c, err, "docker_run_failed")
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (a API) dockerImages(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	return dockerList(a, c, a.Docker.Images)
}

func (a API) dockerPullImage(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	var request struct {
		Image string `json:"image"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	job, err := a.Docker.StartPull(strings.TrimSpace(request.Image))
	if err != nil {
		return dockerError(c, err, "docker_pull_failed")
	}
	a.dockerAudit(c, "docker.image.pull", "docker_image", job.Image, `{"image":`+strconv.Quote(job.Image)+`}`)
	return c.Status(fiber.StatusAccepted).JSON(job)
}

func (a API) dockerPullJobs(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	return c.JSON(a.Docker.PullJobs())
}

func (a API) dockerRemoveImage(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	id := strings.TrimSpace(c.Params("id"))
	force := c.QueryBool("force", false)
	ctx, cancel := context.WithTimeout(c.UserContext(), 25*time.Second)
	defer cancel()
	if err := a.Docker.RemoveImage(ctx, id, force); err != nil {
		return dockerError(c, err, "docker_image_remove_failed")
	}
	a.dockerAudit(c, "docker.image.remove", "docker_image", id, `{"force":`+strconv.FormatBool(force)+`}`)
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) dockerVolumes(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	return dockerList(a, c, a.Docker.Volumes)
}

func (a API) dockerCreateVolume(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	var request struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	if err := a.Docker.CreateVolume(ctx, strings.TrimSpace(request.Name)); err != nil {
		return dockerError(c, err, "docker_volume_create_failed")
	}
	a.dockerAudit(c, "docker.volume.create", "docker_volume", request.Name, "{}")
	return c.SendStatus(fiber.StatusCreated)
}

func (a API) dockerRemoveVolume(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	name := c.Params("name")
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	if err := a.Docker.RemoveVolume(ctx, name); err != nil {
		return dockerError(c, err, "docker_volume_remove_failed")
	}
	a.dockerAudit(c, "docker.volume.remove", "docker_volume", name, "{}")
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) dockerNetworks(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	return dockerList(a, c, a.Docker.Networks)
}

func (a API) dockerRemoveNetwork(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	id := strings.ToLower(c.Params("id"))
	ctx, cancel := context.WithTimeout(c.UserContext(), 15*time.Second)
	defer cancel()
	if err := a.Docker.RemoveNetwork(ctx, id); err != nil {
		return dockerError(c, err, "docker_network_remove_failed")
	}
	a.dockerAudit(c, "docker.network.remove", "docker_network", id, "{}")
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) dockerPrune(c *fiber.Ctx) error {
	if a.Docker == nil {
		return a.dockerUnavailable(c)
	}
	var request struct {
		Kind string `json:"kind"`
	}
	if err := c.BodyParser(&request); err != nil {
		return fiber.ErrBadRequest
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 28*time.Second)
	defer cancel()
	report, err := a.Docker.Prune(ctx, request.Kind)
	if err != nil {
		return dockerError(c, err, "docker_prune_failed")
	}
	a.dockerAudit(c, "docker.prune", "docker", request.Kind, `{"kind":`+strconv.Quote(request.Kind)+`,"deleted":`+strconv.Itoa(report.Deleted)+`}`)
	return c.JSON(report)
}
