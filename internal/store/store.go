package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // pure-Go драйвер: не требует cgo, живёт в alpine

	"github.com/WOLFraMix/pricepulse/internal/model"
)

// Store — доступ к SQLite. Один *sql.DB безопасно используют
// много горутин: драйвер сам управляет пулом соединений.
type Store struct {
	db *sql.DB
}

// New открывает базу и создаёт схему, если её ещё нет.
// WAL-режим (write-ahead log): читатели не блокируют писателя —
// для нашего профиля «много чтений, редкие записи» идеально.
func New(path string) (*Store, error) {
	// Гарантируем существование каталога.
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// busy_timeout: если две горутины пишут одновременно, вторая
	// подождёт до 5 секунд вместо мгновенной ошибки SQLITE_BUSY.
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, err
	}

	if _, err := db.Exec(`
CREATE TABLE IF NOT EXISTS products (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	url          TEXT NOT NULL,
	source       TEXT NOT NULL,
	title        TEXT NOT NULL DEFAULT '',
	target_price REAL NOT NULL,
	active       INTEGER NOT NULL DEFAULT 1,
	created_at   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS price_points (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	product_id  INTEGER NOT NULL REFERENCES products(id),
	price       REAL NOT NULL,
	currency    TEXT NOT NULL DEFAULT 'RUB',
	checked_at  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_points_product
	ON price_points(product_id, checked_at);
`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// CreateProduct добавляет товар и возвращает его id.
// lastInsertId — стандартный механизм AUTOINCREMENT.
func (s *Store) CreateProduct(p model.Product) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO products (url, source, title, target_price, active, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.URL, p.Source, p.Title, p.TargetPrice, p.Active, time.Now().Unix(),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Products возвращает все товары. Активные — то, что проверяют воркеры.
func (s *Store) Products(activeOnly bool) ([]model.Product, error) {
	q := `SELECT id, url, source, title, target_price, active, created_at FROM products`
	if activeOnly {
		q += ` WHERE active = 1`
	}
	rows, err := s.db.Query(q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Инициализируем слайс сразу: пустой результат уедет в JSON
	// как [], а не null — клиенту приятнее.
	out := []model.Product{}
	for rows.Next() {
		var p model.Product
		var active int64
		var created int64
		if err := rows.Scan(&p.ID, &p.URL, &p.Source, &p.Title,
			&p.TargetPrice, &active, &created); err != nil {
			return nil, err
		}
		p.Active = active == 1
		p.CreatedAt = time.Unix(created, 0)
		out = append(out, p)
	}
	return out, rows.Err()
}

// AddPricePoint пишет точку истории цен.
func (s *Store) AddPricePoint(point model.PricePoint) error {
	_, err := s.db.Exec(
		`INSERT INTO price_points (product_id, price, currency, checked_at)
		 VALUES (?, ?, ?, ?)`,
		point.ProductID, point.Price, point.Currency, point.CheckedAt.Unix(),
	)
	return err
}

// History возвращает историю цен товара, начиная со свежих.
func (s *Store) History(productID int64, limit int) ([]model.PricePoint, error) {
	// Нормализация лимита: 0 или отрицательный = «хочу всё разумное
	// по умолчанию». Без этого limit=-1 в SQL означал бы «без лимита»,
	// а для гигантской истории это дорогой ответ.
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := s.db.Query(
		`SELECT product_id, price, currency, checked_at
		 FROM price_points WHERE product_id = ?
		 ORDER BY checked_at DESC LIMIT ?`,
		productID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.PricePoint{}
	for rows.Next() {
		var p model.PricePoint
		var checked int64
		if err := rows.Scan(&p.ProductID, &p.Price, &p.Currency, &checked); err != nil {
			return nil, err
		}
		p.CheckedAt = time.Unix(checked, 0)
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetTitle обновляет название товара (заполняется при первой проверке).
func (s *Store) SetTitle(productID int64, title string) error {
	_, err := s.db.Exec(`UPDATE products SET title = ? WHERE id = ?`, title, productID)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

// LastPrice возвращает цену последней записанной точки товара
// или false, если истории ещё нет. Вызывается ДО вставки новой точки —
// это и есть «прошлая цена» для алерта о снижении.
func (s *Store) LastPrice(productID int64) (float64, bool, error) {
	var price float64
	err := s.db.QueryRow(
		`SELECT price FROM price_points WHERE product_id = ?
		 ORDER BY checked_at DESC LIMIT 1`, productID,
	).Scan(&price)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return price, true, nil
}

// DeleteProduct выключает товар: active = 0. Историю не трогаем —
// она может пригодиться, если товар вернут. Планировщик товары с
// active=0 не публикует, воркер их больше не получает.
func (s *Store) DeleteProduct(id int64) error {
	res, err := s.db.Exec(`UPDATE products SET active = 0 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("product %d not found", id)
	}
	return nil
}
