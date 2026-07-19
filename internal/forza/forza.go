// Package forza reads Forza's "Data Out" UDP telemetry (Horizon 4/5/6 Dash
// format, 324-byte packet) and maps a packet into the glow.moe Forza snapshot.
// It's a passive UDP listener - Forza broadcasts a packet per physics tick to a
// local port (default 5300) once "Data Out" is enabled in the game's telemetry
// settings. Cross-platform (Linux native/Proton or Windows).
//
// The byte offsets mirror Melocet/Forza-Horizon-6-Discord-RPC, verified in-game.
package forza

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
)

// car_names.json maps CarOrdinal → display name (638 FH6 cars, from the ONYX
// game-asset scan). Embedded so the binary is self-contained.
//
//go:embed car_names.json
var carNamesJSON []byte

var carNames = func() map[string]string {
	m := map[string]string{}
	_ = json.Unmarshal(carNamesJSON, &m)
	return m
}()

func carName(ordinal int) string {
	return carNames[fmt.Sprintf("%d", ordinal)]
}

// PacketLen is the Forza Horizon Dash packet size (Sled 232 + pad + Dash).
const PacketLen = 324

// Tire is one wheel's readout (FL/FR/RL/RR).
type Tire struct {
	Pos     string  `json:"pos"`
	Temp    float64 `json:"temp"`
	Surface string  `json:"surface"`
	Wear    float64 `json:"wear"`
}

// Car is the current vehicle summary.
type Car struct {
	Name       string  `json:"name"`
	Class      string  `json:"class"`
	PerfIndex  int     `json:"perfIndex"`
	Drivetrain string  `json:"drivetrain"`
	Cylinders  int     `json:"cylinders"`
	Fuel       float64 `json:"fuel"`     // 0..1
	Distance   string  `json:"distance"` // km, one decimal
}

// Snapshot matches the site's ForzaSnapshot (src/lib/forza/types.ts).
type Snapshot struct {
	Game      string `json:"game"`   // "forza"
	GameID    string `json:"gameId"` // "fh6"
	Racing    bool   `json:"racing"`
	UpdatedAt int64  `json:"updatedAt"`

	Position    int    `json:"position"`
	LapNum      int    `json:"lapNum"`
	TotalLaps   int    `json:"totalLaps"`
	CurLapTime  string `json:"curLapTime"`
	LastLapTime string `json:"lastLapTime"`
	BestLapTime string `json:"bestLapTime"`
	TotalTime   string `json:"totalTime"`

	Speed     int     `json:"speed"` // mph
	Gear      string  `json:"gear"`
	RPM       int     `json:"rpm"`
	MaxRPM    int     `json:"maxRpm"`
	Steer     float64 `json:"steer"`
	Handbrake bool    `json:"handbrake"`
	Throttle  float64 `json:"throttle"`
	Brake     float64 `json:"brake"`
	Clutch    float64 `json:"clutch"`

	Car   Car    `json:"car"`
	Tires []Tire `json:"tires"`

	// Ordinal is kept for a car-name lookup (not in the site type; used for RPC).
	Ordinal int `json:"-"`
	PowerW  float64 `json:"-"`
}

var classNames = map[int32]string{0: "D", 1: "C", 2: "B", 3: "A", 4: "S1", 5: "S2", 6: "X"}
var drivetrainNames = map[int32]string{0: "FWD", 1: "RWD", 2: "AWD"}

func f32(b []byte, off int) float32 {
	return math.Float32frombits(binary.LittleEndian.Uint32(b[off:]))
}
func i32(b []byte, off int) int32 { return int32(binary.LittleEndian.Uint32(b[off:])) }

func lap(sec float32) string {
	if sec <= 0 {
		return "--:--.---"
	}
	m := int(sec) / 60
	rest := sec - float32(m*60)
	return fmt.Sprintf("%d:%06.3f", m, rest)
}

// Parse maps a Forza Dash packet into a Snapshot; ok is false on a short packet.
func Parse(data []byte, gameID string) (*Snapshot, bool) {
	if len(data) < PacketLen {
		return nil, false
	}
	rpm := f32(data, 16)
	maxRPM := f32(data, 8)
	ordinal := i32(data, 212)
	class := i32(data, 216)
	pi := i32(data, 220)
	drivetrain := i32(data, 224)
	cyl := i32(data, 228)

	speedMps := f32(data, 256)
	powerW := f32(data, 260)
	fuel := f32(data, 276)
	distM := f32(data, 280)
	bestLap := f32(data, 284)
	lastLap := f32(data, 288)
	curLap := f32(data, 292)
	raceTime := f32(data, 296)
	lapNum := binary.LittleEndian.Uint16(data[300:])
	pos := data[302]
	throttle := data[303]
	brake := data[304]
	clutch := data[305]
	handbrake := data[306]
	gearB := data[307]
	steer := int8(data[308])

	gear := "N"
	switch {
	case gearB == 0:
		gear = "R"
	case gearB == 11:
		gear = "N"
	default:
		gear = fmt.Sprintf("%d", gearB)
	}

	tirePos := []string{"FL", "FR", "RL", "RR"}
	tires := make([]Tire, 4)
	for i := range tires {
		tires[i] = Tire{Pos: tirePos[i]}
	}

	return &Snapshot{
		Game:        "forza",
		GameID:      gameID,
		Racing:      i32(data, 0) == 1,
		Position:    int(pos),
		LapNum:      int(lapNum),
		CurLapTime:  lap(curLap),
		LastLapTime: lap(lastLap),
		BestLapTime: lap(bestLap),
		TotalTime:   lap(raceTime),
		Speed:       int(math.Round(float64(speedMps) * 2.2369363)), // m/s → mph
		Gear:        gear,
		RPM:         int(rpm),
		MaxRPM:      int(maxRPM),
		Steer:       float64(steer) / 127.0,
		Handbrake:   handbrake != 0,
		Throttle:    float64(throttle) / 255.0,
		Brake:       float64(brake) / 255.0,
		Clutch:      float64(clutch) / 255.0,
		Car: Car{
			Name:       carName(int(ordinal)),
			Class:      classNames[class],
			PerfIndex:  int(pi),
			Drivetrain: orElse(drivetrainNames[drivetrain], "-"),
			Cylinders:  int(cyl),
			Fuel:       clamp01(float64(fuel)),
			Distance:   fmt.Sprintf("%.1f", float64(distM)/1000.0),
		},
		Tires:   tires,
		Ordinal: int(ordinal),
		PowerW:  float64(powerW),
	}, true
}

func orElse(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
