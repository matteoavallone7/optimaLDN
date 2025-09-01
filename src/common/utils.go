package common

import "time"

type UserRequest struct {
	UserID     string    `json:"userID"`
	StartPoint string    `json:"startPoint"`
	EndPoint   string    `json:"endPoint"`
	Departure  time.Time `json:"departure"`
}

type RouteLookup struct {
	UserID  string `json:"userID"`
	RouteID string `json:"routeID"`
}

type UserSavedRoute struct {
	RouteID       string   `json:"routeID"`
	UserID        string   `json:"userID"`
	StartPoint    string   `json:"startPoint"`
	EndPoint      string   `json:"endPoint"`
	TransportMode string   `json:"transportMode"`
	Stops         int      `json:"stops"`
	EstimatedTime int      `json:"estimatedTime"`
	LineNames     []string `json:"lineNames"`
	StopsNames    []string `json:"stopsNames"`
}

type ActiveRoute struct {
	UserID  string   `dynamodbav:"userID" json:"userID"`
	LineIDs []string `dynamodbav:"lineIDs" json:"lineIDs"`
}

type RouteResult struct {
	From    string  `json:"from"`
	To      string  `json:"to"`
	Score   float64 `json:"score"`
	Summary string  `json:"summary"`
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
	StopPoints []StopPointRef `json:"stopPoints"`
}

type StopPointRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ChosenRoute struct {
	UserID        string     `dynamodbav:"userID"`
	TotalDuration int        `dynamodbav:"totalDuration"`
	Description   string     `dynamodbav:"description"`
	Legs          []RouteLeg `dynamodbav:"legs"`
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
	StopIDs     []string `json:"stopIds"`
}

type TimeBandCrowding struct {
	TimeBand             string  `json:"timeBand"`
	PercentageOfBaseLine float64 `json:"percentageOfBaseLine"`
}

type CrowdingResp struct {
	Naptan         string             `json:"naptan"`
	DayOfWeek      string             `json:"dayOfWeek"`
	AmPeakTimeBand string             `json:"amPeakTimeBand"`
	PmPeakTimeBand string             `json:"pmPeakTimeBand"`
	TimeBands      []TimeBandCrowding `json:"timeBands"`
}

type Status int

const (
	StatusPending Status = iota
	StatusDone
	StatusError
)

type SavedResp struct {
	UserID string `json:"userID"`
	Status Status `json:"status"`
}

type NewRequest struct {
	UserID string `json:"userID"`
	Reason string `json:"reason"`
}

type Auth struct {
	UserID   string `json:"userID"`
	Password string `json:"password"`
}
