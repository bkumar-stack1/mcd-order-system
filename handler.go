package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// 構造体定義
type RequestItem struct {
	MenuName  string `json:"menuName"`
	UnitPrice int    `json:"unitPrice"`
	Quantity  int    `json:"quantity"`
	Subtotal  int    `json:"subtotal"`
}

type CreateOrderRequest struct {
	MessageType string        `json:"messageType"`
	TerminalNo  string        `json:"terminalNo"`
	TotalAmount int           `json:"totalAmount"`
	Items       []RequestItem `json:"items"`
}

type BoardKitchenRequest struct {
	TerminalNo  string `json:"terminalNo"`
	MessageType string `json:"messageType"`
	OrderNo     string `json:"orderNo,omitempty"`
}

type UpdateStatusRequest struct {
	OrderStatus string `json:"orderStatus"`
}

// CORS 対応ミドルウェア
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// 共通ログ出力ヘルパー
func logAPIExchange(apiName, method, url string, reqBody []byte, respStatus int, respBody []byte) {
	logger.Printf("[API入電文] [%s] %s %s | Body: %s", apiName, method, url, string(bytes.ReplaceAll(reqBody, []byte("\n"), []byte(""))))
	logger.Printf("[API出電文] [%s] Status: %d | Body: %s", apiName, respStatus, string(bytes.ReplaceAll(respBody, []byte("\n"), []byte(""))))
}

// エラー応答共通処理
func respondWithError(w http.ResponseWriter, r *http.Request, apiName string, status int, msg string, reqBody []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{"result": "NG", "message": msg}
	respBytes, _ := json.Marshal(resp)
	w.Write(respBytes)
	logAPIExchange(apiName, r.Method, r.URL.String(), reqBody, status, respBytes)
}

// 成功応答共通処理
func respondWithJSON(w http.ResponseWriter, r *http.Request, apiName string, status int, data interface{}, reqBody []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	respBytes, _ := json.Marshal(data)
	w.Write(respBytes)
	logAPIExchange(apiName, r.Method, r.URL.String(), reqBody, status, respBytes)
}

// 3.1 POST /api/orders (注文登録)
func handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	var req CreateOrderRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "JSONパースエラー", bodyBytes)
		return
	}

	// 入力チェックバリデーション
	if req.TerminalNo == "" {
		respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "terminalNoは必須項目です", bodyBytes)
		return
	}
	if req.MessageType != "ORDER_CONFIRM" {
		respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "messageTypeがORDER_CONFIRMではありません", bodyBytes)
		return
	}
	if req.TotalAmount < 1 {
		respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "totalAmountは1以上である必要があります", bodyBytes)
		return
	}
	if len(req.Items) < 1 || len(req.Items) > 5 {
		respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "itemsの件数は1〜5件にする必要があります", bodyBytes)
		return
	}

	menuMap := make(map[string]bool)
	calculatedTotal := 0

	for _, item := range req.Items {
		if item.MenuName == "" {
			respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "menuNameは必須項目です", bodyBytes)
			return
		}
		if item.UnitPrice < 1 {
			respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "unitPriceは1以上である必要があります", bodyBytes)
			return
		}
		if item.Quantity < 1 || item.Quantity > 5 {
			respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "quantityは1〜5である必要があります", bodyBytes)
			return
		}
		if menuMap[item.MenuName] {
			respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "同一注文内でmenuNameの重複は禁止です", bodyBytes)
			return
		}
		menuMap[item.MenuName] = true

		// 小計自動計算と不一致チェック
		expectedSubtotal := item.UnitPrice * item.Quantity
		if item.Subtotal != expectedSubtotal {
			respondWithError(w, r, "CreateOrder", http.StatusBadRequest, fmt.Sprintf("subtotal計算不一致: %s", item.MenuName), bodyBytes)
			return
		}
		calculatedTotal += item.Subtotal
	}

	if req.TotalAmount != calculatedTotal {
		respondWithError(w, r, "CreateOrder", http.StatusBadRequest, "totalAmountと明細小計の合計が一致しません", bodyBytes)
		return
	}

	// 採番と登録処理
	orderNo, err := GenerateOrderNoAndInsert(req.TerminalNo, req.Items)
	if err != nil {
		respondWithError(w, r, "CreateOrder", http.StatusInternalServerError, "DB登録処理エラー", bodyBytes)
		return
	}

	logger.Printf("[DB登録内容] 注文登録成功: OrderNo=%s, TerminalNo=%s, ItemsCount=%d", orderNo, req.TerminalNo, len(req.Items))

	resp := map[string]interface{}{
		"result":      "OK",
		"orderNo":     orderNo,
		"orderStatus": StatusReceived,
		"totalAmount": req.TotalAmount,
		"message":     "注文が正常に受信されました",
	}
	respondWithJSON(w, r, "CreateOrder", http.StatusCreated, resp, bodyBytes)
}

// 3.1 GET /api/orders (注文一覧 & 状態別一覧取得)
func handleListOrders(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	items, err := GetOrdersByStatus(statusFilter)
	if err != nil {
		respondWithError(w, r, "ListOrders", http.StatusInternalServerError, "データ取得エラー", nil)
		return
	}

	// 同一注文番号のオブジェクト集約処理
	type AggregatedOrder struct {
		OrderNo     string      `json:"orderNo"`
		TerminalNo  string      `json:"terminalNo"`
		OrderStatus string      `json:"orderStatus"`
		TotalAmount int         `json:"totalAmount"`
		CreatedAt   string      `json:"createdAt"`
		Items       []OrderItem `json:"items"`
	}

	orderMap := make(map[string]*AggregatedOrder)
	var orderList []string // 順序を保持するためのリスト

	for _, item := range items {
		if _, exists := orderMap[item.OrderNo]; !exists {
			orderMap[item.OrderNo] = &AggregatedOrder{
				OrderNo:     item.OrderNo,
				TerminalNo:  item.TerminalNo,
				OrderStatus: item.OrderStatus,
				TotalAmount: 0,
				CreatedAt:   item.CreatedAt.Format("2006-01-02 15:04:05"),
				Items:       []OrderItem{},
			}
			orderList = append(orderList, item.OrderNo)
		}
		orderMap[item.OrderNo].TotalAmount += item.Subtotal
		orderMap[item.OrderNo].Items = append(orderMap[item.OrderNo].Items, item)
	}

	result := make([]*AggregatedOrder, 0, len(orderList))
	for _, no := range orderList {
		result = append(result, orderMap[no])
	}

	respondWithJSON(w, r, "ListOrders", http.StatusOK, result, nil)
}

// 3.1 GET /api/orders/{orderNo} (注文詳細取得)
func handleGetOrderDetail(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")
	if orderNo == "" {
		respondWithError(w, r, "GetOrderDetail", http.StatusBadRequest, "注文番号が指定されていません", nil)
		return
	}

	items, err := GetOrderDetailsByNo(orderNo)
	if err != nil {
		respondWithError(w, r, "GetOrderDetail", http.StatusInternalServerError, "詳細取得エラー", nil)
		return
	}

	if len(items) == 0 {
		respondWithError(w, r, "GetOrderDetail", http.StatusNotFound, "指定された注文番号が見つかりません", nil)
		return
	}

	respondWithJSON(w, r, "GetOrderDetail", http.StatusOK, items, nil)
}

// 3.1 PUT /api/orders/{orderNo}/status (注文状態更新)
func handleUpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	orderNo := r.PathValue("orderNo")
	bodyBytes, _ := io.ReadAll(r.Body)

	var req UpdateStatusRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		respondWithError(w, r, "UpdateOrderStatus", http.StatusBadRequest, "JSONパースエラー", bodyBytes)
		return
	}

	if req.OrderStatus != StatusReceived && req.OrderStatus != StatusCooking && req.OrderStatus != StatusDelivered {
		respondWithError(w, r, "UpdateOrderStatus", http.StatusBadRequest, "無効なステータスです", bodyBytes)
		return
	}

	if err := UpdateOrderStatusByNo(orderNo, req.OrderStatus); err != nil {
		respondWithError(w, r, "UpdateOrderStatus", http.StatusNotFound, err.Error(), bodyBytes)
		return
	}

	logger.Printf("[DB更新内容] ステータス更新成功: OrderNo=%s, NewStatus=%s", orderNo, req.OrderStatus)

	resp := map[string]string{"result": "OK", "orderNo": orderNo, "orderStatus": req.OrderStatus}
	respondWithJSON(w, r, "UpdateOrderStatus", http.StatusOK, resp, bodyBytes)
}

// 3.2 POST /api/board (フロント掲示板機能)
func handleBoardAction(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	var req BoardKitchenRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		respondWithError(w, r, "BoardAction", http.StatusBadRequest, "JSONパースエラー", bodyBytes)
		return
	}

	if req.MessageType != "BOARD_REQUEST" {
		respondWithError(w, r, "BoardAction", http.StatusBadRequest, "messageTypeがBOARD_REQUESTではありません", bodyBytes)
		return
	}

	// orderNoが指定されていれば受け渡し完了処理(DB状態を「受け渡し済み」に更新)
	if req.OrderNo != "" {
		if err := UpdateOrderStatusByNo(req.OrderNo, StatusDelivered); err != nil {
			respondWithError(w, r, "BoardAction", http.StatusNotFound, "指定された注文が見つかりません", bodyBytes)
			return
		}
		logger.Printf("[DB更新内容] 掲示板契機受け渡し完了: OrderNo=%s, Status=%s", req.OrderNo, StatusDelivered)
	}

	// 最新の掲示板情報を生成してレスポンス
	cookingOrders, err := GetDistinctOrderNosByStatus(StatusReceived)
	if err != nil {
		respondWithError(w, r, "BoardAction", http.StatusInternalServerError, "データ抽出エラー", bodyBytes)
		return
	}

	readyOrders, err := GetDistinctOrderNosByStatus(StatusCooking)
	if err != nil {
		respondWithError(w, r, "BoardAction", http.StatusInternalServerError, "データ抽出エラー", bodyBytes)
		return
	}

	resp := map[string]interface{}{
		"result":        "OK",
		"cookingOrders": cookingOrders,
		"readyOrders":   readyOrders,
	}
	respondWithJSON(w, r, "BoardAction", http.StatusOK, resp, bodyBytes)
}

// 3.3 POST /api/kitchen (厨房機能)
func handleKitchenAction(w http.ResponseWriter, r *http.Request) {
	bodyBytes, _ := io.ReadAll(r.Body)
	var req BoardKitchenRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		respondWithError(w, r, "KitchenAction", http.StatusBadRequest, "JSONパースエラー", bodyBytes)
		return
	}

	if req.MessageType != "KITCHEN_REQUEST" {
		respondWithError(w, r, "KitchenAction", http.StatusBadRequest, "messageTypeがKITCHEN_REQUESTではありません", bodyBytes)
		return
	}

	// orderNoが指定されていれば調理完了処理(DB状態を「調理済み」に更新)
	if req.OrderNo != "" {
		if err := UpdateOrderStatusByNo(req.OrderNo, StatusCooking); err != nil {
			respondWithError(w, r, "KitchenAction", http.StatusNotFound, "指定された注文が見つかりません", bodyBytes)
			return
		}
		logger.Printf("[DB更新内容] 厨房契機調理完了: OrderNo=%s, Status=%s", req.OrderNo, StatusCooking)
	}

	// 「オーダ受信済み」の注文明細を取得
	items, err := GetOrdersByStatus(StatusReceived)
	if err != nil {
		respondWithError(w, r, "KitchenAction", http.StatusInternalServerError, "厨房データ抽出エラー", bodyBytes)
		return
	}

	type KitchenItem struct {
		MenuName string `json:"menuName"`
		Quantity int    `json:"quantity"`
	}
	type KitchenOrder struct {
		OrderNo string        `json:"orderNo"`
		Items   []KitchenItem `json:"items"`
	}

	orderMap := make(map[string]*KitchenOrder)
	var orderList []string

	for _, item := range items {
		if _, exists := orderMap[item.OrderNo]; !exists {
			orderMap[item.OrderNo] = &KitchenOrder{
				OrderNo: item.OrderNo,
				Items:   []KitchenItem{},
			}
			orderList = append(orderList, item.OrderNo)
		}
		orderMap[item.OrderNo].Items = append(orderMap[item.OrderNo].Items, KitchenItem{
			MenuName: item.MenuName,
			Quantity: item.Quantity,
		})
	}

	finalOrders := make([]*KitchenOrder, 0, len(orderList))
	for _, no := range orderList {
		finalOrders = append(finalOrders, orderMap[no])
	}

	resp := map[string]interface{}{
		"result": "OK",
		"orders": finalOrders,
	}
	respondWithJSON(w, r, "KitchenAction", http.StatusOK, resp, bodyBytes)
}