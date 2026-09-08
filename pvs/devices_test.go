package pvs

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseDeviceList(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []Device
		wantErr string
	}{
		{
			name: "parses common fields",
			input: `{"devices":[
				{"SERIAL":"INV001","DEVICE_TYPE":"Inverter","TYPE":"MI","MODEL":"SPR-X22","STATE":"working","STATEDESCR":"Working","DATATIME":"1779680954"}
			]}`,
			want: []Device{
				{Serial: "INV001", DeviceType: "Inverter", Type: "MI", Model: "SPR-X22", State: "working", StateDescr: "Working", DataTime: "1779680954"},
			},
		},
		{
			name: "parses multiple devices",
			input: `{"devices":[
				{"SERIAL":"INV001","DEVICE_TYPE":"Inverter"},
				{"SERIAL":"MTR001","DEVICE_TYPE":"Power Meter"},
				{"SERIAL":"PVS001","DEVICE_TYPE":"PVS"}
			]}`,
			want: []Device{
				{Serial: "INV001", DeviceType: "Inverter"},
				{Serial: "MTR001", DeviceType: "Power Meter"},
				{Serial: "PVS001", DeviceType: "PVS"},
			},
		},
		{
			name:  "empty device list",
			input: `{"devices":[]}`,
			want:  []Device{},
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantErr: "unmarshal device list",
		},
		{
			name:    "invalid device entry",
			input:   `{"devices":[1]}`,
			wantErr: "unmarshal device",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDeviceList([]byte(tt.input))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Len(t, got, len(tt.want))
			for i, w := range tt.want {
				assert.Equal(t, w.Serial, got[i].Serial, "Serial[%d]", i)
				assert.Equal(t, w.DeviceType, got[i].DeviceType, "DeviceType[%d]", i)
				assert.Equal(t, w.Type, got[i].Type, "Type[%d]", i)
				assert.Equal(t, w.Model, got[i].Model, "Model[%d]", i)
				assert.Equal(t, w.State, got[i].State, "State[%d]", i)
				assert.Equal(t, w.StateDescr, got[i].StateDescr, "StateDescr[%d]", i)
				assert.Equal(t, w.DataTime, got[i].DataTime, "DataTime[%d]", i)
			}
		})
	}
}

func TestParseDeviceListRawPreserved(t *testing.T) {
	input := `{"devices":[
		{"SERIAL":"INV001","DEVICE_TYPE":"Inverter","p_3phsum_kw":8.5,"temperature":42.1}
	]}`

	got, err := parseDeviceList([]byte(input))
	require.NoError(t, err)
	require.Len(t, got, 1)

	// Raw should contain the full payload including device-specific fields.
	var raw map[string]any
	require.NoError(t, json.Unmarshal(got[0].Raw, &raw))
	assert.Equal(t, "INV001", raw["SERIAL"])
	assert.Equal(t, 8.5, raw["p_3phsum_kw"])
	assert.Equal(t, 42.1, raw["temperature"])
}

func TestParseDeviceListRawIsIndependentPerDevice(t *testing.T) {
	input := `{"devices":[
		{"SERIAL":"A","x":1},
		{"SERIAL":"B","x":2}
	]}`

	got, err := parseDeviceList([]byte(input))
	require.NoError(t, err)
	require.Len(t, got, 2)

	var a, b map[string]any
	require.NoError(t, json.Unmarshal(got[0].Raw, &a))
	require.NoError(t, json.Unmarshal(got[1].Raw, &b))
	assert.Equal(t, float64(1), a["x"])
	assert.Equal(t, float64(2), b["x"])
}

func TestToInverterRelabelsFabricatedZeroRecord(t *testing.T) {
	// The PVS6 reports an inverter it cannot reach as an all-zero record
	// still labelled "working"; 0 V / 0 Hz cannot be a genuine measurement.
	raw := []byte(`{"SERIAL":"INV9","DEVICE_TYPE":"Inverter","STATE":"working","STATEDESCR":"Working"}`)
	d := Device{Serial: "INV9", DeviceType: "Inverter", State: "working", Raw: raw}
	inv, err := d.ToInverter(time.Now())
	require.NoError(t, err)
	assert.Equal(t, StateUnreachable, inv.State)
	assert.Equal(t, "no contact with inverter", inv.StateDescr)
}

func TestToInverterKeepsGenuineStates(t *testing.T) {
	// Real reports carry grid voltage and frequency even at zero power
	// (dawn, dusk, a reported error) and must keep their claimed state.
	for _, state := range []string{"working", "error"} {
		raw := []byte(`{"SERIAL":"INV9","STATE":"` + state + `","STATEDESCR":"x","p_3phsum_kw":"0","vln_3phavg_v":"238.5","freq_hz":"60.0"}`)
		d := Device{Serial: "INV9", DeviceType: "Inverter", State: state, Raw: raw}
		inv, err := d.ToInverter(time.Now())
		require.NoError(t, err)
		assert.Equal(t, state, inv.State)
	}
}
