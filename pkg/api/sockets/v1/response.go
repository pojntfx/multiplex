package v1

type MPVFloat64Response struct {
	Data float64 `json:"data"`
}

type MPVTrackListResponse struct {
	Data []MPVTrackDescription `json:"data"`
}

type MPVTrackDescription struct {
	ID               int    `json:"id"`
	Type             string `json:"type"`
	ExternalFilename string `json:"external-filename"`
	Lang             string `json:"lang"`
	Title            string `json:"title"`
}

type MPVSuccessResponse struct {
	Data []any `json:"data"`
}

type MPVBoolResponse struct {
	Data bool `json:"data"`
}
