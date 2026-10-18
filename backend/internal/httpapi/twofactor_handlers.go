package httpapi

import (
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/auth"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/database"
	"github.com/kantaevsherhan/mini-ubuntu-server-panel/backend/internal/totp"
	"gorm.io/gorm"
)

const (
	totpIssuer = "Mini Ubuntu Server"
	// maxChallengeAttempts bounds code guesses per password-verified login.
	maxChallengeAttempts = 5
	// pendingEnrollmentTTL limits how long an unconfirmed secret stays usable.
	pendingEnrollmentTTL = 15 * time.Minute
)

var errSecondFactorInvalid = errors.New("second factor invalid")

// challengeGuard tracks failed attempts per login challenge and burns challenges
// after success or too many failures. State is in memory: a restart only forces a
// fresh password login.
type challengeGuard struct {
	mu      sync.Mutex
	entries map[string]challengeEntry
}

type challengeEntry struct {
	attempts int
	burned   bool
	expires  time.Time
}

func newChallengeGuard() *challengeGuard {
	return &challengeGuard{entries: map[string]challengeEntry{}}
}

func (g *challengeGuard) usable(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now()
	for key, entry := range g.entries {
		if now.After(entry.expires) {
			delete(g.entries, key)
		}
	}
	entry := g.entries[id]
	return !entry.burned && entry.attempts < maxChallengeAttempts
}

func (g *challengeGuard) fail(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	entry := g.entries[id]
	entry.attempts++
	entry.expires = time.Now().Add(auth.ChallengeTTL)
	g.entries[id] = entry
}

func (g *challengeGuard) burn(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.entries[id] = challengeEntry{burned: true, expires: time.Now().Add(auth.ChallengeTTL)}
}

func (a API) sealer() (*totp.Sealer, error) {
	return totp.NewSealer(a.Secret)
}

func (a API) twoFactorEnabled(tx *gorm.DB, userID int64) (bool, error) {
	var count int64
	err := tx.Model(&database.UserTOTP{}).Where("user_id = ? AND enabled_at IS NOT NULL AND secret_sealed IS NOT NULL", userID).Count(&count).Error
	return count > 0, err
}

// verifySecondFactor accepts a current TOTP code or an unused recovery code and
// records its use (the TOTP step or the recovery code) so it cannot be replayed.
func (a API) verifySecondFactor(tx *gorm.DB, userID int64, code string) (string, error) {
	var record database.UserTOTP
	if err := tx.Where("user_id = ? AND enabled_at IS NOT NULL", userID).First(&record).Error; err != nil || record.SecretSealed == nil {
		return "", errSecondFactorInvalid
	}
	code = strings.TrimSpace(code)
	if len(code) == totp.Digits {
		sealer, err := a.sealer()
		if err != nil {
			return "", err
		}
		secret, err := sealer.Open(*record.SecretSealed)
		if err != nil {
			return "", err
		}
		step, ok := totp.Verify(secret, code, time.Now(), record.LastStep)
		if !ok {
			return "", errSecondFactorInvalid
		}
		// The conditional update makes concurrent use of the same code fail.
		result := tx.Model(&database.UserTOTP{}).Where("user_id = ? AND last_step < ?", userID, step).Update("last_step", step)
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected == 0 {
			return "", errSecondFactorInvalid
		}
		return "totp", nil
	}
	if totp.NormalizeRecoveryCode(code) == "" {
		return "", errSecondFactorInvalid
	}
	result := tx.Model(&database.UserRecoveryCode{}).Where("user_id = ? AND code_hash = ? AND used_at IS NULL", userID, totp.HashRecoveryCode(code)).Update("used_at", time.Now().UTC())
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", errSecondFactorInvalid
	}
	return "recovery_code", nil
}

func (a API) loginSecondFactor(c *fiber.Ctx) error {
	var request struct {
		ChallengeToken string `json:"mfa_token"`
		Code           string `json:"code"`
	}
	if err := c.BodyParser(&request); err != nil || len(request.ChallengeToken) > 2048 || len(request.Code) > 32 {
		return fiber.ErrBadRequest
	}
	userID, challengeID, err := auth.ParseChallenge(a.Secret, request.ChallengeToken)
	if err != nil || !a.Challenges.usable(challengeID) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "mfa_challenge_invalid"})
	}
	var user database.User
	if err := a.DB.WithContext(c.UserContext()).First(&user, userID).Error; err != nil || !user.IsActive {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "mfa_challenge_invalid"})
	}
	var method string
	err = a.DB.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		method, err = a.verifySecondFactor(tx, user.ID, request.Code)
		return err
	})
	if errors.Is(err, errSecondFactorInvalid) {
		a.Challenges.fail(challengeID)
		database.Audit(a.DB, user.ID, "auth.mfa_failed", "user", strconv.FormatInt(user.ID, 10), "{}", c.IP())
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "mfa_code_invalid"})
	}
	if err != nil {
		return err
	}
	a.Challenges.burn(challengeID)
	return a.completeLogin(c, user, method)
}

func (a API) twoFactorStatus(c *fiber.Ctx) error {
	claims := c.Locals("claims").(*auth.Claims)
	var record database.UserTOTP
	err := a.DB.WithContext(c.UserContext()).Where("user_id = ?", claims.UserID).First(&record).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var remaining int64
	if err := a.DB.WithContext(c.UserContext()).Model(&database.UserRecoveryCode{}).Where("user_id = ? AND used_at IS NULL", claims.UserID).Count(&remaining).Error; err != nil {
		return err
	}
	enabled := record.EnabledAt != nil && record.SecretSealed != nil
	return c.JSON(fiber.Map{"enabled": enabled, "enabled_at": record.EnabledAt, "recovery_codes_remaining": remaining})
}

func (a API) twoFactorSetup(c *fiber.Ctx) error {
	claims := c.Locals("claims").(*auth.Claims)
	var request struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&request); err != nil || len(request.Password) > 1024 {
		return fiber.ErrBadRequest
	}
	var user database.User
	if err := a.DB.WithContext(c.UserContext()).First(&user, claims.UserID).Error; err != nil || !auth.Verify(user.PasswordHash, request.Password) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "current_password_invalid"})
	}
	if enabled, err := a.twoFactorEnabled(a.DB.WithContext(c.UserContext()), user.ID); err != nil {
		return err
	} else if enabled {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "mfa_already_enabled"})
	}
	secret, err := totp.NewSecret()
	if err != nil {
		return err
	}
	sealer, err := a.sealer()
	if err != nil {
		return err
	}
	sealed, err := sealer.Seal(secret)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	record := database.UserTOTP{UserID: user.ID, PendingSecretSealed: &sealed, PendingCreatedAt: &now}
	if err := a.DB.WithContext(c.UserContext()).Save(&record).Error; err != nil {
		return err
	}
	database.Audit(a.DB, user.ID, "auth.mfa.setup", "user", strconv.FormatInt(user.ID, 10), "{}", c.IP())
	return c.JSON(fiber.Map{"secret": secret, "otpauth_uri": totp.URI(totpIssuer, user.Username, secret)})
}

func (a API) twoFactorEnable(c *fiber.Ctx) error {
	claims := c.Locals("claims").(*auth.Claims)
	var request struct {
		Code string `json:"code"`
	}
	if err := c.BodyParser(&request); err != nil || len(request.Code) > 32 {
		return fiber.ErrBadRequest
	}
	var record database.UserTOTP
	err := a.DB.WithContext(c.UserContext()).Where("user_id = ?", claims.UserID).First(&record).Error
	if err != nil || record.PendingSecretSealed == nil || record.PendingCreatedAt == nil || time.Since(*record.PendingCreatedAt) > pendingEnrollmentTTL {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "mfa_setup_required"})
	}
	if record.EnabledAt != nil {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "mfa_already_enabled"})
	}
	sealer, err := a.sealer()
	if err != nil {
		return err
	}
	secret, err := sealer.Open(*record.PendingSecretSealed)
	if err != nil {
		return err
	}
	step, ok := totp.Verify(secret, request.Code, time.Now(), 0)
	if !ok {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "mfa_code_invalid"})
	}
	codes, err := totp.NewRecoveryCodes()
	if err != nil {
		return err
	}
	// Activation, recovery codes and session revocation commit together or not at all.
	err = a.DB.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		if err := tx.Model(&database.UserTOTP{}).Where("user_id = ?", claims.UserID).Updates(map[string]any{"secret_sealed": record.PendingSecretSealed, "pending_secret_sealed": nil, "pending_created_at": nil, "enabled_at": now, "last_step": step}).Error; err != nil {
			return err
		}
		if err := replaceRecoveryCodes(tx, claims.UserID, codes); err != nil {
			return err
		}
		return tx.Model(&database.WebSession{}).Where("user_id = ? AND id <> ? AND revoked_at IS NULL", claims.UserID, claims.ID).Update("revoked_at", now).Error
	})
	if err != nil {
		return err
	}
	database.Audit(a.DB, claims.UserID, "auth.mfa.enable", "user", strconv.FormatInt(claims.UserID, 10), `{"other_sessions_revoked":true}`, c.IP())
	return c.JSON(fiber.Map{"recovery_codes": codes})
}

func replaceRecoveryCodes(tx *gorm.DB, userID int64, codes []string) error {
	if err := tx.Where("user_id = ?", userID).Delete(&database.UserRecoveryCode{}).Error; err != nil {
		return err
	}
	now := time.Now().UTC()
	rows := make([]database.UserRecoveryCode, len(codes))
	for i, code := range codes {
		rows[i] = database.UserRecoveryCode{UserID: userID, CodeHash: totp.HashRecoveryCode(code), CreatedAt: now}
	}
	return tx.Create(&rows).Error
}

type sensitiveRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

// confirmSensitive re-checks the password and a second factor before disabling 2FA
// or rotating recovery codes, so a stolen session alone cannot weaken the account.
// It returns an API error code when the confirmation is rejected.
func (a API) confirmSensitive(tx *gorm.DB, userID int64, request sensitiveRequest) (string, error) {
	var user database.User
	if err := tx.First(&user, userID).Error; err != nil || !auth.Verify(user.PasswordHash, request.Password) {
		return "current_password_invalid", nil
	}
	if _, err := a.verifySecondFactor(tx, userID, request.Code); err != nil {
		if errors.Is(err, errSecondFactorInvalid) {
			return "mfa_code_invalid", nil
		}
		return "", err
	}
	return "", nil
}

// withSensitiveConfirmation runs change inside one transaction after confirmSensitive.
func (a API) withSensitiveConfirmation(c *fiber.Ctx, change func(tx *gorm.DB, userID int64) error) error {
	var request sensitiveRequest
	if err := c.BodyParser(&request); err != nil || len(request.Password) > 1024 || len(request.Code) > 32 {
		return fiber.ErrBadRequest
	}
	claims := c.Locals("claims").(*auth.Claims)
	var rejection string
	err := a.DB.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		code, err := a.confirmSensitive(tx, claims.UserID, request)
		if err != nil {
			return err
		}
		if code != "" {
			rejection = code
			return errSecondFactorInvalid
		}
		return change(tx, claims.UserID)
	})
	if rejection != "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": rejection})
	}
	return err
}

func (a API) twoFactorDisable(c *fiber.Ctx) error {
	if err := a.withSensitiveConfirmation(c, deleteTwoFactor); err != nil || c.Response().StatusCode() != fiber.StatusOK {
		return err
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "auth.mfa.disable", "user", strconv.FormatInt(claims.UserID, 10), "{}", c.IP())
	a.notify("security.mfa_disabled", "warning", "2FA отключена: "+claims.Username, "IP: "+c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}

func (a API) twoFactorRecoveryCodes(c *fiber.Ctx) error {
	codes, err := totp.NewRecoveryCodes()
	if err != nil {
		return err
	}
	err = a.withSensitiveConfirmation(c, func(tx *gorm.DB, userID int64) error {
		return replaceRecoveryCodes(tx, userID, codes)
	})
	if err != nil || c.Response().StatusCode() != fiber.StatusOK {
		return err
	}
	claims := c.Locals("claims").(*auth.Claims)
	database.Audit(a.DB, claims.UserID, "auth.mfa.recovery_codes", "user", strconv.FormatInt(claims.UserID, 10), "{}", c.IP())
	return c.JSON(fiber.Map{"recovery_codes": codes})
}

func deleteTwoFactor(tx *gorm.DB, userID int64) error {
	if err := tx.Where("user_id = ?", userID).Delete(&database.UserRecoveryCode{}).Error; err != nil {
		return err
	}
	return tx.Where("user_id = ?", userID).Delete(&database.UserTOTP{}).Error
}

// resetUserTwoFactor lets an administrator recover a user who lost their device.
// The user's sessions are revoked so the next login starts without 2FA.
func (a API) resetUserTwoFactor(c *fiber.Ctx) error {
	targetID, err := parseID(c.Params("id"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	claims := c.Locals("claims").(*auth.Claims)
	var target database.User
	if err := a.DB.WithContext(c.UserContext()).First(&target, targetID).Error; err != nil {
		return fiber.ErrNotFound
	}
	err = a.DB.WithContext(c.UserContext()).Transaction(func(tx *gorm.DB) error {
		if err := deleteTwoFactor(tx, targetID); err != nil {
			return err
		}
		return tx.Model(&database.WebSession{}).Where("user_id = ? AND revoked_at IS NULL", targetID).Update("revoked_at", time.Now().UTC()).Error
	})
	if err != nil {
		return err
	}
	database.Audit(a.DB, claims.UserID, "user.mfa.reset", "user", strconv.FormatInt(targetID, 10), `{"sessions_revoked":true}`, c.IP())
	a.notify("security.mfa_disabled", "warning", "2FA сброшена: "+target.Username, "Администратор: "+claims.Username+", IP: "+c.IP())
	return c.SendStatus(fiber.StatusNoContent)
}
