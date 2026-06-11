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

// parseFloat converts a PVS6 numeric string to float64.
// An empty string returns (0, nil) since sleeping inverters legitimately omit fields.
// A non-empty unparseable value returns an error.
func parseFloat(field, s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("field %s: cannot parse %q as float: %w", field, s, err)
	}
	return f, nil
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
	var err error
	inv := InverterDevice{
		Serial:     r.Serial,
		State:      r.State,
		StateDescr: r.StateDescr,
		ReceivedAt: receivedAt,
	}
	if inv.PowerKW, err = parseFloat("power_kw", r.PowerKW); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	if inv.LifetimeKWh, err = parseFloat("lifetime_kwh", r.LifetimeKWh); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	if inv.VoltageV, err = parseFloat("voltage_v", r.VoltageV); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	if inv.CurrentA, err = parseFloat("current_a", r.CurrentA); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	if inv.PowerMPPT1KW, err = parseFloat("power_mppt1_kw", r.PowerMPPT1); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	if inv.VoltageMPPT1V, err = parseFloat("voltage_mppt1_v", r.VoltageMPPT1); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	if inv.CurrentMPPT1A, err = parseFloat("current_mppt1_a", r.CurrentMPPT1); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	if inv.TempC, err = parseFloat("temp_c", r.TempC); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	if inv.FreqHz, err = parseFloat("freq_hz", r.FreqHz); err != nil {
		return InverterDevice{}, fmt.Errorf("inverter %s: %w", r.Serial, err)
	}
	return inv, nil
}
