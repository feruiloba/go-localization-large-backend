package model

import "encoding/json"

// Response defines the response to the user
type Response struct {
	ExperimentID        string          `json:"experimentId"`
	SelectedPayloadName string          `json:"selectedPayloadName"`
	Payload             json.RawMessage `json:"payload"`
}
