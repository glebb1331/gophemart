// Package storage реализует слой хранения данных для сервиса Гофермарт.
// Предоставляет интерфейс Storage и его реализацию на основе PostgreSQL.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Ошибки бизнес-логики хранилища.
var (
	// ErrConflict возвращается при попытке создать пользователя с уже существующим логином.
	ErrConflict = errors.New("conflict: login already exists")
	// ErrOrderAlreadyUploadedByUser возвращается, если заказ уже загружен этим пользователем.
	ErrOrderAlreadyUploadedByUser = errors.New("order already uploaded by this user")
	// ErrOrderAlreadyUploadedByOther возвращается, если заказ уже загружен другим пользователем.
	ErrOrderAlreadyUploadedByOther = errors.New("order already uploaded by another user")
	// ErrInsufficientFunds возвращается при недостаточном балансе для списания.
	ErrInsufficientFunds = errors.New("insufficient funds")
)

// Статусы обработки заказа.
const (
	// StatusNew — заказ загружен, но ещё не обработан.
	StatusNew = "NEW"
	// StatusProcessing — заказ в процессе обработки.
	StatusProcessing = "PROCESSING"
	// StatusInvalid — заказ отклонён системой начислений.
	StatusInvalid = "INVALID"
	// StatusProcessed — заказ обработан, начисление выполнено.
	StatusProcessed = "PROCESSED"
)

// Storage определяет интерфейс хранилища данных для сервиса Гофермарт.
type Storage interface {
	CreateUser(ctx context.Context, login, passwordHash string) (int, error)
	GetUserByLogin(ctx context.Context, login string) (int, string, error)
	CreateOrder(ctx context.Context, userID int, orderNum string) error
	GetUserOrders(ctx context.Context, userID int) ([]Order, error)
	GetUserBalance(ctx context.Context, userID int) (Balance, error)
	Withdraw(ctx context.Context, userID int, orderNum string, sum float64) error
	GetUserWithdrawals(ctx context.Context, userID int) ([]Withdrawal, error)
	GetOrdersForProcessing(ctx context.Context, limit int) ([]Order, error)
	ProcessAccrual(ctx context.Context, orderNum string, userID int, status string, accrual float64) error
	Close() error
}

// DatabaseStorage реализует интерфейс Storage с использованием PostgreSQL.
type DatabaseStorage struct {
	db *sql.DB
}

// Order описывает заказ пользователя в системе.
type Order struct {
	// ID — внутренний идентификатор заказа.
	ID int
	// UserID — идентификатор пользователя, загрузившего заказ.
	UserID int
	// Number — номер заказа.
	Number string
	// Status — текущий статус обработки заказа.
	Status string
	// Accrual — начисленные баллы лояльности.
	Accrual float64
	// UploadedAt — время загрузки заказа.
	UploadedAt time.Time
}

// Balance описывает баланс пользователя.
type Balance struct {
	// Current — текущий баланс баллов лояльности.
	Current float64 `json:"current"`
	// Withdrawn — общая сумма списанных баллов за всё время.
	Withdrawn float64 `json:"withdrawn"`
}

// Withdrawal описывает операцию списания баллов.
type Withdrawal struct {
	// Order — номер заказа, в счёт которого произведено списание.
	Order string `json:"order"`
	// Sum — сумма списанных баллов.
	Sum float64 `json:"sum"`
	// ProcessedAt — время обработки списания.
	ProcessedAt time.Time `json:"processed_at"`
}

// NewDatabaseStorage создаёт новое хранилище на основе PostgreSQL.
// Выполняет подключение к БД и применяет миграции.
func NewDatabaseStorage(dsn string) (*DatabaseStorage, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return nil, err
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return nil, err
	}

	return &DatabaseStorage{db: db}, nil
}

// Close закрывает соединение с базой данных.
func (s *DatabaseStorage) Close() error {
	return s.db.Close()
}

// CreateUser создаёт нового пользователя и возвращает его ID.
// Возвращает ErrConflict, если пользователь с таким логином уже существует.
func (s *DatabaseStorage) CreateUser(ctx context.Context, login, passwordHash string) (int, error) {
	var id int
	query := `INSERT INTO users (login, password) VALUES ($1, $2) RETURNING id`
	err := s.db.QueryRowContext(ctx, query, login, passwordHash).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return 0, ErrConflict
		}
		return 0, err
	}
	return id, nil
}

// GetUserByLogin возвращает ID и хеш пароля пользователя по логину.
func (s *DatabaseStorage) GetUserByLogin(ctx context.Context, login string) (int, string, error) {
	var id int
	var passwordHash string
	query := `SELECT id, password FROM users WHERE login = $1`
	err := s.db.QueryRowContext(ctx, query, login).Scan(&id, &passwordHash)
	if err != nil {
		return 0, "", err
	}
	return id, passwordHash, nil
}

// CreateOrder создаёт новый заказ для пользователя.
// Возвращает ErrOrderAlreadyUploadedByUser или ErrOrderAlreadyUploadedByOther
// при дублировании номера заказа.
func (s *DatabaseStorage) CreateOrder(ctx context.Context, userID int, orderNum string) error {
	query := `INSERT INTO orders (user_id, number, status) VALUES ($1, $2, $3)`
	_, err := s.db.ExecContext(ctx, query, userID, orderNum, StatusNew)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			var ownerID int
			checkQuery := `SELECT user_id FROM orders WHERE number = $1`
			err2 := s.db.QueryRowContext(ctx, checkQuery, orderNum).Scan(&ownerID)
			if err2 != nil {
				return err
			}
			if ownerID == userID {
				return ErrOrderAlreadyUploadedByUser
			}
			return ErrOrderAlreadyUploadedByOther
		}
		return err
	}
	return nil
}

// GetUserOrders возвращает список заказов пользователя, отсортированных по времени загрузки (DESC).
func (s *DatabaseStorage) GetUserOrders(ctx context.Context, userID int) ([]Order, error) {
	query := `SELECT number, status, accrual, uploaded_at FROM orders
              WHERE user_id = $1 ORDER BY uploaded_at DESC`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.Number, &o.Status, &o.Accrual, &o.UploadedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

// GetUserBalance возвращает текущий баланс и сумму списаний пользователя.
func (s *DatabaseStorage) GetUserBalance(ctx context.Context, userID int) (Balance, error) {
	var b Balance
	query := `SELECT balance, withdrawn FROM users WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&b.Current, &b.Withdrawn)
	return b, err
}

// Withdraw выполняет списание баллов со счёта пользователя в рамках транзакции.
// Возвращает ErrInsufficientFunds при недостаточном балансе.
func (s *DatabaseStorage) Withdraw(ctx context.Context, userID int, orderNum string, sum float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var balance float64
	checkQuery := `SELECT balance FROM users WHERE id = $1 FOR UPDATE`
	if err := tx.QueryRowContext(ctx, checkQuery, userID).Scan(&balance); err != nil {
		return err
	}
	if balance < sum {
		return ErrInsufficientFunds
	}

	updateQuery := `UPDATE users SET balance = balance - $1, withdrawn = withdrawn + $1 WHERE id = $2`
	if _, err := tx.ExecContext(ctx, updateQuery, sum, userID); err != nil {
		return err
	}

	insertQuery := `INSERT INTO withdrawals (user_id, order_number, sum) VALUES ($1, $2, $3)`
	if _, err := tx.ExecContext(ctx, insertQuery, userID, orderNum, sum); err != nil {
		return err
	}

	return tx.Commit()
}

// GetUserWithdrawals возвращает список списаний пользователя, отсортированных по времени (DESC).
func (s *DatabaseStorage) GetUserWithdrawals(ctx context.Context, userID int) ([]Withdrawal, error) {
	query := `SELECT order_number, sum, processed_at FROM withdrawals
              WHERE user_id = $1 ORDER BY processed_at DESC`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var wds []Withdrawal
	for rows.Next() {
		var w Withdrawal
		if err := rows.Scan(&w.Order, &w.Sum, &w.ProcessedAt); err != nil {
			return nil, err
		}
		wds = append(wds, w)
	}
	return wds, rows.Err()
}

// GetOrdersForProcessing возвращает заказы со статусами NEW и PROCESSING для обработки воркером.
func (s *DatabaseStorage) GetOrdersForProcessing(ctx context.Context, limit int) ([]Order, error) {
	query := `SELECT id, user_id, number, status, accrual, uploaded_at FROM orders
              WHERE status IN ($1, $2) ORDER BY uploaded_at ASC LIMIT $3`
	rows, err := s.db.QueryContext(ctx, query, StatusNew, StatusProcessing, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.UserID, &o.Number, &o.Status, &o.Accrual, &o.UploadedAt); err != nil {
			return nil, err
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

// ProcessAccrual атомарно обновляет статус заказа и начисляет баллы на баланс пользователя.
func (s *DatabaseStorage) ProcessAccrual(ctx context.Context, orderNum string, userID int, status string, accrual float64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	updateOrder := `UPDATE orders SET status = $1, accrual = $2 WHERE number = $3`
	if _, err := tx.ExecContext(ctx, updateOrder, status, accrual, orderNum); err != nil {
		return err
	}

	if status == StatusProcessed && accrual > 0 {
		updateBalance := `UPDATE users SET balance = balance + $1 WHERE id = $2`
		if _, err := tx.ExecContext(ctx, updateBalance, accrual, userID); err != nil {
			return err
		}
	}

	return tx.Commit()
}
