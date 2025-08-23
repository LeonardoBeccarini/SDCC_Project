package entities

type SoilProfile struct {
	ThetaFC        float64 `json:"theta_fc"`
	ThetaWP        float64 `json:"theta_wp"`
	Efficiency     float64 `json:"efficiency"`
	RainEfficiency float64 `json:"rain_efficiency"`
	CooldownHours  int     `json:"cooldown_h"`
}
