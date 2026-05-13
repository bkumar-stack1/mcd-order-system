package main
    var items []map[string]interface{}
package main

import (
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "time"
)
    for rows.Next() {
        var orderNo, terminalNo, orderStatus string
        var itemNo, unitPrice, quantity, subtotal int
        var menuName, createdAt string

        rows.Scan(
            &orderNo,
            &terminalNo,
            &orderStatus,
            &itemNo,
            &menuName,
            &unitPrice,
            &quantity,
            &subtotal,
            &createdAt,
        )

        items = append(items, map[string]interface{}{
            "orderNo": orderNo,
            "terminalNo": terminalNo,
            "orderStatus": orderStatus,
            "itemNo": itemNo,
            "menuName": menuName,
            "unitPrice": unitPrice,
            "quantity": quantity,
            "subtotal": subtotal,
            "createdAt": createdAt,
        })
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(items)
}

func UpdateOrderStatus(w http.ResponseWriter, r *http.Request, orderNo string) {
    if r.Method != http.MethodPut {
        http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
        return
    }

    var req StatusRequest

    json.NewDecoder(r.Body).Decode(&req)

    _, err := db.Exec(`
    UPDATE order_items
    SET order_status = ?
    WHERE order_no = ?
    `, req.OrderStatus, orderNo)

    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    logger.Println("STATUS UPDATE:", orderNo, req.OrderStatus)

    response := map[string]string{
        "result": "OK",
        "message": "status updated",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}