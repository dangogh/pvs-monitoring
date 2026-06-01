package pvs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// InverterDevice holds a single per-inverter reading.
type InverterDevice struct {
	Serial        string    `json:"serial"`
	State         string    `json:"state"`
	StateDescr    string    `json:"state_descr"`
	ReceivedAt    time.Time `json:"received_at"`
	PowerKW       float64   `json:"power_kw"`
	LifetimeKWh   float64   `json:"lifetime_kwh"`
	VoltageV      float64   `json:"voltage_v"`
	CurrentA      float64   `json:"current_a"`
	PowerMPPT1KW  float64   `json:"power_mppt1_kw"`
	VoltageMPPT1V float64   `json:"voltage_mppt1_v"`
	CurrentMPPT1A float64   `json:"current_mppt1_a"`
	TempC         float64   `json:"temp_c"`
	FreqHz        float64   `json:"freq_hz"`
}

// AuxDevice holds a raw reading for non-inverter devices (PVS, Power Meter).
type AuxDevice struct {
	Serial     string
	DeviceType string
	ReceivedAt time.Time
	Payload    json.RawMessage
}

// rawInverter is used for JSON parsing of inverter payloads.
type rawInverter struct {
	Serial       string `json:"SERIAL"`
	State        string `json:"STATE"`
	StateDescr   string `json:"STATEDESCR"`
	PowerKW      string `json:"p_3phsum_kw"`
	LifetimeKWh  string `json:"ltea_3phsum_kwh"`
	VoltageV     string `json:"vln_3phavg_v"`
	CurrentA     string `json:"i_3phsum_a"`
	PowerMPPT1   string `json:"p_mppt1_kw"`
	VoltageMPPT1 string `json:"v_mppt1_v"`
	CurrentMPPT1 string `json:"i_mppt1_a"`
	TempC        string `json:"t_htsnk_degc"`
	FreqHz       string `json:"freq_hz"`
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// Device is the raw device list entry from the PVS6, used during parsing before typed dispatch.
type Device struct {
	Serial     string          `json:"SERIAL"`
	DeviceType string          `json:"DEVICE_TYPE"`
	Type       string          `json:"TYPE"`
	Model      string          `json:"MODEL"`
	State      string          `json:"STATE"`
	StateDescr string          `json:"STATEDESCR"`
	DataTime   string          `json:"DATATIME"`
	Raw        json.RawMessage `json:"-"`
}

type deviceListResponse struct {
	Devices []json.RawMessage `json:"devices"`
}

func parseDeviceList(data []byte) ([]Device, error) {
	var resp deviceListResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal device list: %w", err)
	}
	devices := make([]Device, 0, len(resp.Devices))
	for _, raw := range resp.Devices {
		var d Device
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("unmarshal device: %w", err)
		}
		d.Raw = raw
		devices = append(devices, d)
	}
	return devices, nil
}

// ToInverter parses a Device's Raw payload into an InverterDevice.
func (d Device) ToInverter(receivedAt time.Time) (InverterDevice, error) {
	var r rawInverter
	if err := json.Unmarshal(d.Raw, &r); err != nil {
		return InverterDevice{}, err
	}
	return InverterDevice{
		Serial:        r.Serial,
		State:         r.State,
		StateDescr:    r.StateDescr,
		ReceivedAt:    receivedAt,
		PowerKW:       parseFloat(r.PowerKW),
		LifetimeKWh:   parseFloat(r.LifetimeKWh),
		VoltageV:      parseFloat(r.VoltageV),
		CurrentA:      parseFloat(r.CurrentA),
		PowerMPPT1KW:  parseFloat(r.PowerMPPT1),
		VoltageMPPT1V: parseFloat(r.VoltageMPPT1),
		CurrentMPPT1A: parseFloat(r.CurrentMPPT1),
		TempC:         parseFloat(r.TempC),
		FreqHz:        parseFloat(r.FreqHz),
	}, nil
}
