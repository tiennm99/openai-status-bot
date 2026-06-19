package openai

type Page struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`
}

type OverallStatus struct {
	Description string `json:"description"`
	Indicator   string `json:"indicator"`
}

type Component struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Position  int    `json:"position"`
	PageID    string `json:"page_id"`
	GroupID   string `json:"group_id"`
	Group     bool   `json:"group"`
}

type Summary struct {
	Page       Page          `json:"page"`
	Status     OverallStatus `json:"status"`
	Components []Component   `json:"components"`
	Incidents  []Incident    `json:"incidents"`
}

type Incident struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Status          string           `json:"status"`
	CreatedAt       string           `json:"created_at"`
	UpdatedAt       string           `json:"updated_at"`
	ResolvedAt      string           `json:"resolved_at"`
	Impact          string           `json:"impact"`
	Shortlink       string           `json:"shortlink"`
	PageID          string           `json:"page_id"`
	IncidentUpdates []IncidentUpdate `json:"incident_updates"`
}

type IncidentUpdate struct {
	ID         string `json:"id"`
	Body       string `json:"body"`
	CreatedAt  string `json:"created_at"`
	DisplayAt  string `json:"display_at"`
	IncidentID string `json:"incident_id"`
	Status     string `json:"status"`
	UpdatedAt  string `json:"updated_at"`
}

type IncidentsResponse struct {
	Page      Page       `json:"page"`
	Incidents []Incident `json:"incidents"`
}
