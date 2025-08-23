package entities

type Stage string

const (
	StageEmergence  Stage = "Emergence"
	StageMaxRooting Stage = "MaxRooting"
	StageSenescence Stage = "Senescence"
	StageMaturity   Stage = "Maturity"
)

type StageParams struct {
	Kc      float64 `json:"kc"`
	SMT     float64 `json:"smt_pct"`       // %
	RootZmm float64 `json:"root_depth_mm"` // mm
}
