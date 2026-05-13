package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type OrderItem struct {
	MenuName  string `json:"menuName"`
	UnitPrice int    `json:"unitPrice"`
	Quantity  int    `json:"quantity"`
}

type OrderRequest struct {
	TerminalNo  string      `json:"terminalNo"`
	MessageType string      `json:"messageType"`
	TotalAmount int         `json:"totalAmount"`
	Items       []OrderItem `json:"items"`
}

func writeLog(msg string) {
	_ = os.MkdirAll("logs", 0755)
	f, _ := os.OpenFile("logs/order.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		defer f.Close()
		f.WriteString(fmt.Sprintf("[%s] %s\n", time.Now().Format("2006-01-02 15:04:05"), msg))
	}
}

func OrdersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method == "POST" {
		var req OrderRequest
		json.NewDecoder(r.Body).Decode(&req)

		orderNo := fmt.Sprintf("%s-%03d", time.Now().Format("0102"), getCountToday(time.Now().Format("0102"))+1)
		calcTotal := 0
		for i, item := range req.Items {
			subtotal := item.UnitPrice * item.Quantity
			calcTotal += subtotal
			db.Exec("INSERT INTO order_items (order_no, terminal_no, order_status, item_no, menu_name, unit_price, quantity, subtotal) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
				orderNo, req.TerminalNo, "オーダー受信", i+1, item.MenuName, item.UnitPrice, item.Quantity, subtotal)
		}

		res := map[string]interface{}{
			"result": "OK", "orderNo": orderNo, "orderStatus": "オーダー受信", "message": "注文を受け付けました",
		}
		writeLog(fmt.Sprintf("NEW ORDER: %s", orderNo))
		json.NewEncoder(w).Encode(res)

	} else if r.Method == "GET" {
		status := r.URL.Query().Get("status")
		query := "SELECT DISTINCT order_no, order_status FROM order_items"
		var rows *sql.Rows
		var err error
		if status != "" {
			rows, err = db.Query(query+" WHERE order_status = ?", status)
		} else {
			rows, err = db.Query(query)
		}
		if err != nil { return }
		defer rows.Close()

		list := []map[string]string{}
		for rows.Next() {
			var no, st string
			rows.Scan(&no, &st)
			list = append(list, map[string]string{"orderNo": no, "orderStatus": st})
		}
		json.NewEncoder(w).Encode(list)
	}
}

func OrderDetailHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 { return }
	orderNo := parts[3]

	if r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/status") {
		var body struct{ OrderStatus string `json:"orderStatus"` }
		json.NewDecoder(r.Body).Decode(&body)
		db.Exec("UPDATE order_items SET order_status = ? WHERE order_no = ?", body.OrderStatus, orderNo)
		writeLog(fmt.Sprintf("UPDATE: %s to %s", orderNo, body.OrderStatus))
		return
	}

	rows, _ := db.Query("SELECT menu_name, unit_price, quantity, subtotal FROM order_items WHERE order_no = ?", orderNo)
	defer rows.Close()
	var items []interface{}
	for rows.Next() {
		var m string
		var u, q, s int
		rows.Scan(&m, &u, &q, &s)
		items = append(items, map[string]interface{}{"menuName": m, "unitPrice": u, "quantity": q, "subtotal": s})
	}
	json.NewEncoder(w).Encode(items)
}