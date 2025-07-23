package common

import "time"

type UserRequest struct {
	UserID        string
	StartPoint    string
	EndPoint      string
	DepartureDate string // yyyyMMdd format
	DepartureTime time.Time
}

type UserSavedRoute struct {
	RouteID       string
	UserID        string
	StartPoint    string
	EndPoint      string
	TransportMode string
	Stops         int
	EstimatedTime int
	LineNames     []string
	StopsNames    []string
}

type ActiveRoute struct {
	UserID  string
	LineIDs []string
}

type RouteResult struct {
	From    string
	To      string
	Score   float64
	Summary string
}

type TfLAlert struct {
	LineName          string    `json:"lineName"`
	ModeName          string    `json:"modeName"`
	StatusDescription string    `json:"statusDescription,omitempty"` // Omit if empty
	Reason            string    `json:"reason,omitempty"`            // Omit if empty
	SeverityDrop      float64   `json:"severityDrop,omitempty"`      // Omit if 0
	Timestamp         time.Time `json:"timestamp"`
}

// NotificationPayload wraps the alert type and a list of alerts.
type NotificationPayload struct {
	AlertType   string     `json:"alertType"` // e.g., "CriticalDelay", "SuddenServiceWorsening"
	Alerts      []TfLAlert `json:"alerts"`
	GeneratedAt time.Time  `json:"generatedAt"`
}

type TfLLineStatusResponse []TfLLine

type TfLLine struct {
	ID           string
	Name         string
	ModeName     string
	LineStatuses []LineStatus
}

type LineStatus struct {
	StatusSeverity            int
	StatusSeverityDescription string
	Reason                    string
	IsPlanned                 bool
	ValidityPeriods           []ValidityPeriod
}

type ValidityPeriod struct {
	FromDate string
	ToDate   string
	IsNow    bool
}

type TFLJourneyResponse struct {
	Journeys []TFLJourney `json:"journeys"`
}

type TFLJourney struct {
	Duration int      `json:"duration"`
	Legs     []TFLLeg `json:"legs"`
}

type TFLLeg struct {
	DepartureTime  string        `json:"departureTime"`
	ArrivalTime    string        `json:"arrivalTime"`
	DeparturePoint StopPoint     `json:"departurePoint"`
	ArrivalPoint   StopPoint     `json:"arrivalPoint"`
	Instruction    Instruction   `json:"instruction"`
	RouteOptions   []RouteOption `json:"routeOptions"`
	Path           Path          `json:"path"`
	Mode           Mode          `json:"mode"`
}

type StopPoint struct {
	CommonName string `json:"commonName"`
	NaptanID   string `json:"naptanId"`
}

type Instruction struct {
	Summary  string `json:"summary"`
	Detailed string `json:"detailed"`
}

type RouteOption struct {
	Name           string         `json:"name"`
	LineIdentifier LineIdentifier `json:"lineIdentifier"`
}

type LineIdentifier struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Mode struct {
	Name string `json:"name"`
}

type Path struct {
	StopPoints []StopPointRef
}

type StopPointRef struct {
	ID   string
	Name string
}

type ChosenRoute struct {
	TotalDuration int
	Description   string
	Legs          []RouteLeg
}

type RouteLeg struct {
	From        string   `json:"from"`
	FromID      string   `json:"fromId"`
	To          string   `json:"to"`
	ToID        string   `json:"toId"`
	Mode        string   `json:"mode"`
	StartTime   string   `json:"startTime"`
	EndTime     string   `json:"endTime"`
	Description string   `json:"description"`
	LineName    string   `json:"lineName"`
	LineID      string   `json:"lineId"`
	Stops       []string `json:"stops"`
	StopIDs     []string
}

type TimeBandCrowding struct {
	TimeBand             string
	PercentageOfBaseLine float64
}

type CrowdingResp struct {
	Naptan         string
	DayOfWeek      string
	AmPeakTimeBand string
	PmPeakTimeBand string
	TimeBands      []TimeBandCrowding
}

type Status int

const (
	StatusPending Status = iota
	StatusDone
	StatusError
)

type SavedResp struct {
	UserID string
	Status Status
}

type NewRequest struct {
	UserID string
	Reason string
}
