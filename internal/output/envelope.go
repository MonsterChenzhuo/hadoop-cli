package output

import (
	"encoding/json"
	"io"
)

type HostResult struct {
	Host      string `json:"host"`
	OK        bool   `json:"ok"`
	ElapsedMs int64  `json:"elapsed_ms"`
	Message   string `json:"message,omitempty"`
}

type EnvelopeError struct {
	Code    string `json:"code"`
	Host    string `json:"host,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Envelope struct {
	Command       string         `json:"command"`
	OK            bool           `json:"ok"`
	Summary       map[string]any `json:"summary,omitempty"`
	Hosts         []HostResult   `json:"hosts,omitempty"`
	Error         *EnvelopeError `json:"error,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
	InventoryPath string         `json:"inventory_path,omitempty"`
}

func NewEnvelope(command string) *Envelope {
	return &Envelope{Command: command, OK: true}
}

func (e *Envelope) WithSummary(s map[string]any) *Envelope {
	e.Summary = s
	return e
}

func (e *Envelope) AddHost(r HostResult) *Envelope {
	if !r.OK {
		e.OK = false
	}
	e.Hosts = append(e.Hosts, r)
	return e
}

func (e *Envelope) WithError(err EnvelopeError) *Envelope {
	e.OK = false
	e.Error = &err
	return e
}

func (e *Envelope) WithRunID(id string) *Envelope {
	e.RunID = id
	return e
}

func (e *Envelope) Write(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}
