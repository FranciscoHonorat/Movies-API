package shared

type MoviePublisherMessage struct {
	CorrelationID string `json:"correlation_id"`
	Title         string `json:"title"`
	Year          string `json:"year"`
}
