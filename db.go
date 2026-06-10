package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

const (
	StatusReceived  = "オーダ受信済み"
	StatusCooking   = "調理済み"
	StatusDelivered = "受け渡し済み"
)

// OrderItem DBマッピング用構造体
type OrderItem struct {
	ID          int64     `json:"id"`
	OrderNo     string    `json:"orderNo"`
	TerminalNo  string    `json:"terminalNo"`
	OrderStatus string    `json:"orderStatus"`
	ItemNo      int       `json:"itemNo"`
	MenuName    string    `json:"menuName"`
	UnitPrice   int       `json:"unitPrice"`
	Quantity    int       `json:"quantity"`
	Subtotal    int       `json:"subtotal"`
	CreatedAt   time.Time `json:"createdAt"`
}

// InitDB データベースを初期化し、テーブルを作成する
func InitDB() error {
	var err error
	// 同時書き込み対策のタイムアウト設定を付与
	db, err = sql.Open("sqlite3", "order.db?_busy_timeout=5000")
	if err != nil {
		return err
	}

	// 同時書き込み競合を防ぐためのコネクション数制限
	db.SetMaxOpenConns(1)

	// テーブル作成クエリ
	schema := `
	CREATE TABLE IF NOT EXISTS order_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		order_no TEXT NOT NULL,
		terminal_no TEXT NOT NULL,
		order_status TEXT NOT NULL,
		item_no INTEGER NOT NULL,
		menu_name TEXT NOT NULL,
		unit_price INTEGER NOT NULL,
		quantity INTEGER NOT NULL,
		subtotal INTEGER NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(schema)
	return err
}

// CloseDB データベース接続を閉じる
func CloseDB() {
	if db != nil {
		db.Close()
	}
}

// GenerateOrderNoAndInsert 採番とインサートを同一トランザクション内で実行
func GenerateOrderNoAndInsert(terminalNo string, items []RequestItem) (string, error) {
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// 今日の日付 (MMDD) を取得
	todayStr := time.Now().Format("0102")

	// 同日内の最新のオーダー番号を取得して連番を採番 (デッドロック防止のため同一トランザクション内)
	var lastOrderNo sql.NullString
	query := `SELECT order_no FROM order_items WHERE order_no LIKE ? ORDER BY id DESC LIMIT 1`
	err = tx.QueryRow(query, todayStr+"-%").Scan(&lastOrderNo)

	nextSeq := 1
	if err == nil && lastOrderNo.Valid {
		var mmdd string
		var seq int
		_, errParse := fmt.Sscanf(lastOrderNo.String, "%4s-%3d", &mmdd, &seq)
		if errParse == nil {
			nextSeq = seq + 1
		}
	}

	orderNo := fmt.Sprintf("%s-%03d", todayStr, nextSeq)

	// 明細データの登録
	insertQuery := `
	INSERT INTO order_items (order_no, terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	for i, item := range items {
		itemNo := i + 1
		_, err = tx.Exec(insertQuery, orderNo, terminalNo, StatusReceived, itemNo, item.MenuName, item.UnitPrice, item.Quantity, item.Subtotal)
		if err != nil {
			return "", err
		}
	}

	// トランザクションコミット
	if err := tx.Commit(); err != nil {
		return "", err
	}

	return orderNo, nil
}

// GetOrdersByStatus ステータス指定（空文字の場合は全件）で注文を取得
func GetOrdersByStatus(status string) ([]OrderItem, error) {
	var rows *sql.Rows
	var err error

	if status != "" {
		query := `SELECT id, order_no, terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal, created_at 
		          FROM order_items WHERE order_status = ? ORDER BY id ASC`
		rows, err = db.Query(query, status)
	} else {
		query := `SELECT id, order_no, terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal, created_at 
		          FROM order_items ORDER BY id ASC`
		rows, err = db.Query(query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OrderItem
	for rows.Next() {
		var item OrderItem
		err := rows.Scan(&item.ID, &item.OrderNo, &item.TerminalNo, &item.OrderStatus, &item.ItemNo, &item.MenuName, &item.UnitPrice, &item.Quantity, &item.Subtotal, &item.CreatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// GetOrderDetailsByNo 注文番号から明細全件を取得
func GetOrderDetailsByNo(orderNo string) ([]OrderItem, error) {
	query := `SELECT id, order_no, terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal, created_at 
	          FROM order_items WHERE order_no = ? ORDER BY item_no ASC`
	rows, err := db.Query(query, orderNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []OrderItem
	for rows.Next() {
		var item OrderItem
		err := rows.Scan(&item.ID, &item.OrderNo, &item.TerminalNo, &item.OrderStatus, &item.ItemNo, &item.MenuName, &item.UnitPrice, &item.Quantity, &item.Subtotal, &item.CreatedAt)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// UpdateOrderStatusByNo 注文ステータスを更新する
func UpdateOrderStatusByNo(orderNo string, nextStatus string) error {
	query := `UPDATE order_items SET order_status = ? WHERE order_no = ?`
	result, err := db.Exec(query, nextStatus, orderNo)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("対象のオーダー番号が見つかりません: %s", orderNo)
	}
	return nil
}

// GetDistinctOrderNosByStatus 特定ステータスの注文番号一覧を一意に取得
func GetDistinctOrderNosByStatus(status string) ([]string, error) {
	query := `SELECT DISTINCT order_no FROM order_items WHERE order_status = ? ORDER BY id ASC`
	rows, err := db.Query(query, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orderNos []string
	for rows.Next() {
		var no string
		if err := rows.Scan(&no); err != nil {
			return nil, err
		}
		orderNos = append(orderNos, no)
	}
	return orderNos, nil
}