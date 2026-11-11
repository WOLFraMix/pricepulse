package model

import "time"

// Product — отслеживаемый товар. Один товар = одна URL + ожидаемая цена.
type Product struct {
	ID          int64
	URL         string  // страница товара
	Source      string  // ключ профиля из sources.yaml: "avito", "dns", ...
	Title       string  // человеческое название (с первой проверки)
	TargetPrice float64 // уведомляем, когда текущая <= этого порога
	Active      bool    // отключенный товар не проверяется
	CreatedAt   time.Time
}

// PricePoint — одна точка истории цен. Из них строится график.
type PricePoint struct {
	ProductID int64
	Price     float64
	Currency  string
	CheckedAt time.Time
}

// CheckTask — сообщение в RabbitMQ: «проверь товар в такое-то время».
// TargetPrice кладём в задачу: воркер должен сам решить, пора ли алертить,
// не бегая за товаром в базу — задача самодостаточна.
type CheckTask struct {
	ProductID   int64     `json:"product_id"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	TargetPrice float64   `json:"target_price"`
	DueAt       time.Time `json:"due_at"` // когда пора проверять
}
