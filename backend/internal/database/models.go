package database

import "time"

type User struct {
	ID                 int64  `gorm:"primaryKey"`
	Username           string `gorm:"uniqueIndex;not null"`
	DisplayName        string `gorm:"not null"`
	PasswordHash       string `gorm:"not null"`
	Role               string `gorm:"not null"`
	IsActive           bool   `gorm:"not null"`
	MustChangePassword bool   `gorm:"not null"`
	SystemUsername     *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastLoginAt        *time.Time
}

type AuditEvent struct {
	ID          int64 `gorm:"primaryKey"`
	ActorUserID *int64
	Action      string `gorm:"not null"`
	TargetType  string `gorm:"not null"`
	TargetID    *string
	DetailsJSON string `gorm:"not null"`
	IPAddress   string
	CreatedAt   time.Time
}

type WebSession struct {
	ID         string `gorm:"primaryKey"`
	UserID     int64  `gorm:"index;not null"`
	IPAddress  string `gorm:"not null"`
	UserAgent  string `gorm:"not null"`
	CreatedAt  time.Time
	LastSeenAt time.Time
	ExpiresAt  time.Time `gorm:"index"`
	RevokedAt  *time.Time
}

type TelegramSetting struct {
	ID                    int64 `gorm:"primaryKey"`
	Enabled               bool
	APIBaseURL            string
	RequestTimeoutSeconds int
	RetryCount            int
	UpdatedAt             time.Time
}

type TelegramRecipient struct {
	ID             int64 `gorm:"primaryKey"`
	TelegramUserID *int64
	TelegramChatID int64 `gorm:"uniqueIndex;not null"`
	DisplayName    *string
	Enabled        bool
	ReceiveAlerts  bool
	ReceiveAudit   bool
	ReceiveUpdates bool
	CreatedAt      time.Time
}

type NotificationEvent struct {
	ID          int64 `gorm:"primaryKey"`
	EventKey    string
	Severity    string
	PayloadJSON string
	DedupKey    *string `gorm:"uniqueIndex"`
	Status      string
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	ResolvedAt  *time.Time
}

type NotificationDelivery struct {
	ID            int64 `gorm:"primaryKey"`
	EventID       int64 `gorm:"index"`
	RecipientID   int64
	Status        string `gorm:"index"`
	Attempts      int
	LastError     *string
	NextAttemptAt *time.Time `gorm:"index"`
	DeliveredAt   *time.Time
	CreatedAt     time.Time
}

type NotificationRule struct {
	EventKey              string `gorm:"primaryKey"`
	Enabled               bool
	Severity              string
	CooldownSeconds       int
	RepeatIntervalSeconds int
	SendRecovery          bool
	UpdatedAt             time.Time
}

type NotificationRuleRecipient struct {
	EventKey    string `gorm:"primaryKey"`
	RecipientID int64  `gorm:"primaryKey"`
}

type NotificationRuleState struct {
	EventKey        string `gorm:"primaryKey"`
	Active          bool
	ActiveDedupKey  *string
	LastEventID     *int64
	LastTriggeredAt *time.Time
	LastNotifiedAt  *time.Time
	ResolvedAt      *time.Time
}

type MetricSample struct {
	ID               int64 `gorm:"primaryKey"`
	SampledAt        time.Time
	CPUPercent       float64
	MemoryPercent    float64
	MemoryUsedBytes  uint64
	MemoryTotalBytes uint64
}

type SchemaMigration struct {
	Version   string `gorm:"primaryKey"`
	AppliedAt time.Time
}
