package api

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/WOLFraMix/pricepulse/internal/model"
	"github.com/WOLFraMix/pricepulse/internal/store"
)

// Server держит зависимости и собирает маршруты.
type Server struct {
	store *store.Store
	r     *gin.Engine
}

// New — сборка: движок, маршруты, middleware.
func New(st *store.Store) *Server {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.SetTrustedProxies(nil)
	// gin.Default() = движок + два middleware: Logger (каждый запрос
	// в лог с временем и статусом) и Recovery (паника в обработчике
	// не роняет сервис — превращается в HTTP 500).

	s := &Server{store: st, r: r}

	api := r.Group("/api")
	{
		api.POST("/products", s.createProduct)
		api.GET("/products", s.listProducts)
		api.GET("/products/:id/history", s.history)
		api.DELETE("/products/:id", s.deleteProduct)
	}
	return s
}

// createProduct — тело запроса биндим в структуру и валидируем.
type createProductReq struct {
	URL         string  `json:"url" binding:"required"`
	Source      string  `json:"source" binding:"required"`
	Title       string  `json:"title"`
	TargetPrice float64 `json:"target_price" binding:"required,gt=0"`
}

func (s *Server) createProduct(c *gin.Context) {
	var req createProductReq
	// ShouldBindJSON: парсит тело в структуру, проверяет binding-теги.
	// При ошибке — 400 с описанием, обработчик не падает.
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// SSRF-защита: воркер будет скачивать этот URL. Разрешаем только
	// абсолютные http/https-ссылки — без схемы и хоста запрос не имеет
	// смысла, а прочие схемы (file:, gopher: и т.п.) отклоняем сразу.
	if u, err := url.Parse(req.URL); err != nil ||
		(u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid url"})
		return
	}

	p := model.Product{
		URL:         req.URL,
		Source:      req.Source,
		Title:       req.Title,
		TargetPrice: req.TargetPrice,
		Active:      true,
	}
	id, err := s.store.CreateProduct(p)
	if err != nil {
		log.Printf("create product: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	// 201 Created — и сразу возвращаем созданное с id.
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (s *Server) listProducts(c *gin.Context) {
	// ?active=1 — фильтр для планировщика; без параметра — все товары.
	activeOnly := c.Query("active") == "1"

	products, err := s.store.Products(activeOnly)
	if err != nil {
		log.Printf("list products: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	// Store инициализирует слайс пустым [], поэтому здесь null не появится.
	c.JSON(http.StatusOK, products)
}

func (s *Server) history(c *gin.Context) {
	// c.Param("id") всегда строка (путь): конвертируем и валидируем.
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// ?limit= — опционален; Store нормализует 0/мусор к дефолту 100.
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
		return
	}

	points, err := s.store.History(id, limit)
	if err != nil {
		log.Printf("history %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, points)
}

func (s *Server) Run(addr string) error {
	return s.r.Run(addr)
}

func (s *Server) deleteProduct(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := s.store.DeleteProduct(id); err != nil {
		// Не найден — 404; прочее (например, сбой БД) — 500 без
		// утечки текста внутренней ошибки клиенту.
		if errors.Is(err, store.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		log.Printf("delete product %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}
