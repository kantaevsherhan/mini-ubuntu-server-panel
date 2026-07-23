package httpapi

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/OWNER/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/OWNER/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/OWNER/mini-ubuntu-server-panel/backend/internal/systemusers"
)

type API struct { DB *sql.DB; Secret string; Version string }
type loginRequest struct { Username string `json:"username"`; Password string `json:"password"` }
type createUserRequest struct { Username string `json:"username"`; DisplayName string `json:"display_name"`; Password string `json:"password"`; Role string `json:"role"`; SystemUsername string `json:"system_username"` }

func (a API) Register(app *fiber.App) {
	api := app.Group("/api/v1")
	api.Get("/health", func(c *fiber.Ctx) error { if err:=a.DB.Ping(); err!=nil{return c.Status(503).JSON(fiber.Map{"status":"error"})}; return c.JSON(fiber.Map{"status":"ok","version":a.Version}) })
	api.Post("/auth/login", a.login)
	secured := api.Group("", a.authorize)
	secured.Get("/me", func(c *fiber.Ctx) error{return c.JSON(c.Locals("claims"))})
	secured.Get("/dashboard", a.dashboard)
	secured.Get("/users", a.users)
	secured.Post("/users", a.requireRole("admin"), a.createUser)
	secured.Get("/system-users", a.requireRole("admin","operator"), func(c *fiber.Ctx) error { u,e:=systemusers.List(); if e!=nil{return e}; return c.JSON(u) })
	secured.Get("/telegram/settings", a.requireRole("admin"), a.telegramSettings)
	secured.Put("/telegram/settings", a.requireRole("admin"), a.updateTelegramSettings)
	secured.Get("/audit", a.requireRole("admin"), a.audit)
}

func (a API) login(c *fiber.Ctx) error {
	var r loginRequest; if err:=c.BodyParser(&r); err!=nil{return fiber.ErrBadRequest}
	var id int64; var hash,role string; var active,must bool
	err:=a.DB.QueryRow(`SELECT id,password_hash,role,is_active,must_change_password FROM users WHERE username=?`,r.Username).Scan(&id,&hash,&role,&active,&must)
	if err!=nil || !active || !auth.Verify(hash,r.Password) { database.Audit(a.DB,nil,"auth.login_failed","user",r.Username,"{}",c.IP()); return c.Status(401).JSON(fiber.Map{"error":"invalid_credentials"}) }
	token,err:=auth.Sign(a.Secret,id,r.Username,role); if err!=nil{return err}
	_,_=a.DB.Exec(`UPDATE users SET last_login_at=?,updated_at=? WHERE id=?`,time.Now().UTC(),time.Now().UTC(),id)
	database.Audit(a.DB,id,"auth.login","user",strconv.FormatInt(id,10),"{}",c.IP())
	return c.JSON(fiber.Map{"access_token":token,"token_type":"Bearer","must_change_password":must})
}
func (a API) authorize(c *fiber.Ctx) error { h:=c.Get("Authorization"); if !strings.HasPrefix(h,"Bearer "){return fiber.ErrUnauthorized}; cl,e:=auth.Parse(a.Secret,strings.TrimPrefix(h,"Bearer ")); if e!=nil{return fiber.ErrUnauthorized}; c.Locals("claims",cl); return c.Next() }
func (a API) requireRole(roles ...string) fiber.Handler { return func(c *fiber.Ctx) error {cl:=c.Locals("claims").(*auth.Claims); for _,r:=range roles{if cl.Role==r{return c.Next()}}; return fiber.ErrForbidden} }
func (a API) dashboard(c *fiber.Ctx) error { var users,events int; _=a.DB.QueryRow(`SELECT count(*) FROM users WHERE is_active=1`).Scan(&users); _=a.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE status='pending'`).Scan(&events); return c.JSON(fiber.Map{"hostname":hostname(),"panel_users":users,"pending_notifications":events,"status":"online"}) }
func hostname() string { return "ubuntu-server" }
func (a API) users(c *fiber.Ctx) error { rows,e:=a.DB.Query(`SELECT id,username,display_name,role,is_active,system_username,created_at,last_login_at FROM users ORDER BY username`);if e!=nil{return e};defer rows.Close();out:=[]fiber.Map{};for rows.Next(){var id int;var u,d,r string;var active bool;var sys,last sql.NullString;var created time.Time;if e=rows.Scan(&id,&u,&d,&r,&active,&sys,&created,&last);e!=nil{return e};out=append(out,fiber.Map{"id":id,"username":u,"display_name":d,"role":r,"is_active":active,"system_username":sys.String,"created_at":created,"last_login_at":last.String})};return c.JSON(out) }
func (a API) createUser(c *fiber.Ctx) error {var r createUserRequest;if e:=c.BodyParser(&r);e!=nil{return fiber.ErrBadRequest};if len(r.Username)<3||len(r.Password)<12{return c.Status(422).JSON(fiber.Map{"error":"username_or_password_invalid"})};if r.Role!="admin"&&r.Role!="operator"&&r.Role!="viewer"{return fiber.ErrBadRequest};h,e:=auth.Hash(r.Password);if e!=nil{return e};now:=time.Now().UTC();res,e:=a.DB.Exec(`INSERT INTO users(username,display_name,password_hash,role,system_username,created_at,updated_at) VALUES(?,?,?,?,NULLIF(?,''),?,?)`,r.Username,r.DisplayName,h,r.Role,r.SystemUsername,now,now);if e!=nil{return c.Status(409).JSON(fiber.Map{"error":"username_exists"})};id,_:=res.LastInsertId();cl:=c.Locals("claims").(*auth.Claims);database.Audit(a.DB,cl.UserID,"user.create","user",fmt.Sprint(id),`{"password":"hidden"}`,c.IP());return c.Status(201).JSON(fiber.Map{"id":id})}
func (a API) telegramSettings(c *fiber.Ctx) error {var enabled bool;var url string;var timeout,retry,count int;e:=a.DB.QueryRow(`SELECT enabled,api_base_url,request_timeout_seconds,retry_count FROM telegram_settings WHERE id=1`).Scan(&enabled,&url,&timeout,&retry);if e!=nil{return e};_=a.DB.QueryRow(`SELECT count(*) FROM telegram_recipients WHERE enabled=1`).Scan(&count);return c.JSON(fiber.Map{"enabled":enabled,"api_base_url":url,"request_timeout_seconds":timeout,"retry_count":retry,"recipient_count":count,"token_configured":false})}
func (a API) updateTelegramSettings(c *fiber.Ctx) error {var r struct{Enabled bool `json:"enabled"`; APIBaseURL string `json:"api_base_url"`; RequestTimeout int `json:"request_timeout_seconds"`; RetryCount int `json:"retry_count"`};if e:=c.BodyParser(&r);e!=nil{return fiber.ErrBadRequest};if !strings.HasPrefix(r.APIBaseURL,"https://")&&!strings.HasPrefix(r.APIBaseURL,"http://127.0.0.1"){return c.Status(422).JSON(fiber.Map{"error":"invalid_api_url"})};_,e:=a.DB.Exec(`UPDATE telegram_settings SET enabled=?,api_base_url=?,request_timeout_seconds=?,retry_count=?,updated_at=? WHERE id=1`,r.Enabled,r.APIBaseURL,r.RequestTimeout,r.RetryCount,time.Now().UTC());if e!=nil{return e};cl:=c.Locals("claims").(*auth.Claims);database.Audit(a.DB,cl.UserID,"telegram.settings.update","telegram","1",`{"token_value":"hidden"}`,c.IP());return c.SendStatus(204)}
func (a API) audit(c *fiber.Ctx) error {rows,e:=a.DB.Query(`SELECT id,action,target_type,COALESCE(target_id,''),details_json,created_at FROM audit_events ORDER BY id DESC LIMIT 200`);if e!=nil{return e};defer rows.Close();out:=[]fiber.Map{};for rows.Next(){var id int;var action,target,targetID,details string;var created time.Time;if e=rows.Scan(&id,&action,&target,&targetID,&details,&created);e!=nil{return e};out=append(out,fiber.Map{"id":id,"action":action,"target_type":target,"target_id":targetID,"details":details,"created_at":created})};return c.JSON(out)}
