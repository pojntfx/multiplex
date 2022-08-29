package v1

type ResponseFloat64 struct {
	Data float64 `json:"data"`
}

type ResponseBool struct {
	Data bool `json:"data"`
}

type ResponseTrackList struct {
	Data []ResponseTrackDescription `json:"data"`
}

type ResponseTrackDescription struct {
	ID               int    `json:"id"`
	Type             string `json:"type"`
	ExternalFilename string `json:"external-filename"`
	Lang             string `json:"lang"`
	Title            string `json:"title"`
}

type ResponseSuccess struct {
	Data []any `json:"data"`
}
