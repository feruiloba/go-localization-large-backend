package model

// Response defines the response to the user
type Response struct {
	ExperimentID        string `json:"experimentId"`
	SelectedPayloadName string `json:"selectedPayloadName"`
	Payload             string `json:"payload"`
}
