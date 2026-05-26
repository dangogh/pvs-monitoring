package pvs

import (
	"encoding/json"
	"fmt"
)

// Device represents a single entry from the PVS6 device list.
// Raw holds the full JSON payload for fields specific to each device type.
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
