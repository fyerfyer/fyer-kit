package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/fyerfyer/fyer-kit/pool"
)

// SQLConnection 实现 Connection 接口，包装 SQL 数据库连接
type SQLConnection struct {
	db        *sql.DB
	conn      *sql.Conn
	tx        *sql.Tx
	lastUsed  time.Time
	mutex     sync.RWMutex
	closed    bool
	healthSQL string
}

// Close 实现 Connection 接口的关闭方法
func (c *SQLConnection) Close() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil
	}

	var err error
	// 如果有活跃的事务，回滚它
	if c.tx != nil {
		err = c.tx.Rollback()
		c.tx = nil
	}

	// 关闭连接
	if c.conn != nil {
		if closeErr := c.conn.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		c.conn = nil
	}

	c.closed = true
	return err
}

// Raw 返回底层的 SQL 连接
func (c *SQLConnection) Raw() interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	// 如果有事务，返回事务
	if c.tx != nil {
		return c.tx
	}

	// 如果有连接，返回连接
	if c.conn != nil {
		return c.conn
	}

	// 否则返回数据库
	return c.db
}

// IsAlive 检查连接是否仍然可用
func (c *SQLConnection) IsAlive() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	if c.closed {
		return false
	}

	// 如果有连接，使用健康检查 SQL 进行测试
	if c.conn != nil && c.healthSQL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		var result int
		err := c.conn.QueryRowContext(ctx, c.healthSQL).Scan(&result)
		return err == nil
	}

	// 如果有数据库但没有连接，使用 Ping 检查
	if c.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		return c.db.PingContext(ctx) == nil
	}

	return false
}

// ResetState 准备连接以供重用
func (c *SQLConnection) ResetState() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 如果有活跃的事务，回滚它
	if c.tx != nil {
		if err := c.tx.Rollback(); err != nil {
			return fmt.Errorf("failed to rollback transaction on reset: %w", err)
		}
		c.tx = nil
	}

	c.lastUsed = time.Now()
	c.closed = false
	return nil
}

// BeginTx 开始一个事务
func (c *SQLConnection) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.closed {
		return nil, fmt.Errorf("connection is closed")
	}

	// 如果已经有事务，返回错误
	if c.tx != nil {
		return nil, fmt.Errorf("transaction already started")
	}

	var err error
	if c.conn != nil {
		c.tx, err = c.conn.BeginTx(ctx, opts)
	} else if c.db != nil {
		c.tx, err = c.db.BeginTx(ctx, opts)
	} else {
		return nil, fmt.Errorf("no database connection available")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	return c.tx, nil
}

// CommitTx 提交当前事务
func (c *SQLConnection) CommitTx() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.tx == nil {
		return fmt.Errorf("no transaction to commit")
	}

	err := c.tx.Commit()
	c.tx = nil
	return err
}

// RollbackTx 回滚当前事务
func (c *SQLConnection) RollbackTx() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.tx == nil {
		return fmt.Errorf("no transaction to rollback")
	}

	err := c.tx.Rollback()
	c.tx = nil
	return err
}

// SQLDBConfig 定义 SQL 连接池配置
type SQLDBConfig struct {
	// 驱动名称
	DriverName string

	// 数据源名称（连接字符串）
	DataSourceName string

	// 健康检查 SQL
	HealthCheckSQL string

	// 连接池设置
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration

	// 是否使用单独的连接而不是数据库池
	UseIndividualConns bool

	// 初始化函数，在创建连接时调用
	InitFunc func(*sql.DB) error
}

// DefaultSQLConfig 返回默认的 SQL 配置
func DefaultSQLConfig(driverName, dataSourceName string) *SQLDBConfig {
	healthCheckSQL := "SELECT 1"

	// 根据不同的数据库类型选择不同的健康检查 SQL
	switch driverName {
	case "postgres", "pgx":
		healthCheckSQL = "SELECT 1"
	case "mysql":
		healthCheckSQL = "SELECT 1"
	case "sqlite3":
		healthCheckSQL = "SELECT 1"
	case "sqlserver":
		healthCheckSQL = "SELECT 1"
	case "oracle", "godror":
		healthCheckSQL = "SELECT 1 FROM DUAL"
	}

	return &SQLDBConfig{
		DriverName:         driverName,
		DataSourceName:     dataSourceName,
		HealthCheckSQL:     healthCheckSQL,
		MaxOpenConns:       10,
		MaxIdleConns:       5,
		ConnMaxLifetime:    30 * time.Minute,
		ConnMaxIdleTime:    10 * time.Minute,
		UseIndividualConns: false,
	}
}

// SQLConnectionFactory 创建 SQL 连接的工厂
type SQLConnectionFactory struct {
	db     *sql.DB
	config *SQLDBConfig
}

// NewSQLConnectionFactory 创建一个新的 SQL 连接工厂
func NewSQLConnectionFactory(config *SQLDBConfig) (*SQLConnectionFactory, error) {
	if config == nil {
		return nil, fmt.Errorf("SQL configuration cannot be nil")
	}

	db, err := sql.Open(config.DriverName, config.DataSourceName)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 配置连接池
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// 立即验证连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// 如果提供了初始化函数，执行它
	if config.InitFunc != nil {
		if err := config.InitFunc(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("database initialization failed: %w", err)
		}
	}

	return &SQLConnectionFactory{
		db:     db,
		config: config,
	}, nil
}

// Create 实现 ConnectionFactory 接口，创建一个新的 SQL 连接
func (f *SQLConnectionFactory) Create(ctx context.Context) (pool.Connection, error) {
	var conn *sql.Conn
	var err error

	// 如果需要单独的连接而不是使用池连接
	if f.config.UseIndividualConns {
		conn, err = f.db.Conn(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get individual connection: %w", err)
		}
	}

	sqlConn := &SQLConnection{
		db:        f.db,
		conn:      conn,
		lastUsed:  time.Now(),
		healthSQL: f.config.HealthCheckSQL,
	}

	return sqlConn, nil
}

// Close 关闭连接工厂，释放资源
func (f *SQLConnectionFactory) Close() error {
	if f.db != nil {
		return f.db.Close()
	}
	return nil
}

// WithExistingDB 使用现有的 SQL 数据库作为连接源
func WithExistingDB(db *sql.DB, healthSQL string) pool.ConnectionFactory {
	return &existingDBFactory{
		db:        db,
		healthSQL: healthSQL,
	}
}

// existingDBFactory 是包装现有 DB 的工厂
type existingDBFactory struct {
	db        *sql.DB
	healthSQL string
}

// Create 从现有数据库创建连接
func (f *existingDBFactory) Create(ctx context.Context) (pool.Connection, error) {
	return &SQLConnection{
		db:        f.db,
		lastUsed:  time.Now(),
		healthSQL: f.healthSQL,
	}, nil
}

// SQLExecutor 定义执行 SQL 操作的接口
type SQLExecutor interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// SQLPoolHelper SQL 连接池助手
type SQLPoolHelper struct {
	pool pool.Pool
}

// NewSQLPoolHelper 创建一个新的 SQL 连接池助手
func NewSQLPoolHelper(p pool.Pool) *SQLPoolHelper {
	return &SQLPoolHelper{
		pool: p,
	}
}

// WithConnection 使用数据库连接执行操作
func (h *SQLPoolHelper) WithConnection(ctx context.Context, fn func(SQLExecutor) error) error {
	conn, err := h.pool.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection from pool: %w", err)
	}
	defer conn.Close()

	// 获取底层 SQL 执行器（可能是 *sql.DB, *sql.Conn, 或 *sql.Tx）
	executor, ok := conn.Raw().(SQLExecutor)
	if !ok {
		return fmt.Errorf("invalid SQL executor type")
	}

	err = fn(executor)
	return h.pool.Put(conn, err)
}

// WithTransaction 使用事务执行操作
func (h *SQLPoolHelper) WithTransaction(ctx context.Context, opts *sql.TxOptions, fn func(*sql.Tx) error) error {
	conn, err := h.pool.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get connection from pool: %w", err)
	}
	defer conn.Close()

	// 尝试获取 SQLConnection 以使用 BeginTx
	sqlConn, ok := conn.(*SQLConnection)
	if !ok {
		return fmt.Errorf("not a SQL connection")
	}

	// 开始事务
	tx, err := sqlConn.BeginTx(ctx, opts)
	if err != nil {
		return h.pool.Put(conn, err)
	}

	// 执行事务操作
	err = fn(tx)
	if err != nil {
		// 如果操作失败，回滚事务
		if rbErr := sqlConn.RollbackTx(); rbErr != nil {
			err = fmt.Errorf("operation failed (%v) and transaction rollback failed: %v", err, rbErr)
		}
		return h.pool.Put(conn, err)
	}

	// 提交事务
	if err := sqlConn.CommitTx(); err != nil {
		return h.pool.Put(conn, fmt.Errorf("failed to commit transaction: %w", err))
	}

	return h.pool.Put(conn, nil)
}

// Execute 执行单个 SQL 查询并处理结果
func (h *SQLPoolHelper) Execute(ctx context.Context, query string, args []interface{}, fn func(SQLExecutor) error) error {
	return h.WithConnection(ctx, func(executor SQLExecutor) error {
		return fn(executor)
	})
}

// Query 执行查询并处理结果集
func (h *SQLPoolHelper) Query(ctx context.Context, query string, args []interface{}, fn func(*sql.Rows) error) error {
	return h.WithConnection(ctx, func(executor SQLExecutor) error {
		rows, err := executor.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		return fn(rows)
	})
}

// QueryRow 执行查询并处理单行结果
func (h *SQLPoolHelper) QueryRow(ctx context.Context, query string, args []interface{}, fn func(*sql.Row) error) error {
	return h.WithConnection(ctx, func(executor SQLExecutor) error {
		row := executor.QueryRowContext(ctx, query, args...)
		return fn(row)
	})
}

// Exec 执行不返回行的查询
func (h *SQLPoolHelper) Exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	var result sql.Result
	err := h.WithConnection(ctx, func(executor SQLExecutor) error {
		var err error
		result, err = executor.ExecContext(ctx, query, args...)
		return err
	})
	return result, err
}
