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
	ID          int
	PlantID     int
	PhotoPath   string
	PhotoData   []byte
	PhotoMime   string
	HealthScore int
	Diagnosis   string
	CareTips    string
	CreatedAt   time.Time
}
