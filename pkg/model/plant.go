package model

import "time"

type Plant struct {
	ID         int
	Name       string
	Species    string
	CommonName string
	CreatedAt  time.Time
}

type Assessment struct {
	ID             int
	PlantID        int
	PhotoPath      string
	PhotoData      []byte
	PhotoMime      string
	HealthScore    int
	Confidence     string
	Diagnosis      string
	CareTips       string
	Foliage        int
	Hydration      int
	PestRisk       int
	Vitality       int
	Urgent         string
	SeasonalAdvice string
	CreatedAt      time.Time
}
