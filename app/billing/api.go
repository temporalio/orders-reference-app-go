package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/temporalio/orders-reference-app-go/app/internal/temporalutil"
	"go.temporal.io/sdk/client"
)

// TaskQueue is the default task queue for the Billing system.
const TaskQueue = "billing"

// Item represents an item being ordered.
type Item struct {
	SKU      string `json:"sku"`
	Quantity int32  `json:"quantity"`
}

// ChargeInput is the input for the Charge workflow.
type ChargeInput struct {
	CustomerID string `json:"customerId"`
	Reference  string `json:"orderReference"`
	Items      []Item `json:"items"`
}

// ChargeResult is the result for the Charge workflow.
type ChargeResult struct {
	InvoiceReference string `json:"invoiceReference"`
	SubTotal         int32  `json:"subTotal"`
	Shipping         int32  `json:"shipping"`
	Tax              int32  `json:"tax"`

	Success  bool   `json:"success"`
	AuthCode string `json:"authCode"`
}

// GenerateInvoiceInput is the input for the GenerateInvoice activity.
type GenerateInvoiceInput struct {
	CustomerID string `json:"customerId"`
	Reference  string `json:"orderReference"`
	Items      []Item `json:"items"`
}

// GenerateInvoiceResult is the result for the GenerateInvoice activity.
type GenerateInvoiceResult struct {
	InvoiceReference string `json:"invoiceReference"`
	SubTotal         int32  `json:"subTotal"`
	Shipping         int32  `json:"shipping"`
	Tax              int32  `json:"tax"`
}

// ChargeCustomerInput is the input for the ChargeCustomer activity.
type ChargeCustomerInput struct {
	CustomerID string `json:"customerId"`
	Reference  string `json:"reference"`
	Charge     int32  `json:"charge"`
}

// ChargeCustomerResult is the result for the GenerateInvoice activity.
type ChargeCustomerResult struct {
	Success  bool   `json:"success"`
	AuthCode string `json:"authCode"`
}

type handlers struct {
	temporal client.Client
}

// RunServer runs a Billing API HTTP server on the given port.
func RunServer(ctx context.Context, port int) error {
	clientOptions, err := temporalutil.CreateClientOptionsFromEnv()
	if err != nil {
		return fmt.Errorf("failed to create client options: %v", err)
	}

	c, err := client.Dial(clientOptions)
	if err != nil {
		return fmt.Errorf("client error: %v", err)
	}
	defer c.Close()

	srv := &http.Server{
		Addr:    fmt.Sprintf("0.0.0.0:%d", port),
		Handler: Router(c),
	}

	fmt.Printf("Listening on http://0.0.0.0:%d\n", port)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case <-ctx.Done():
		srv.Close()
	case err = <-errCh:
		return err
	}

	return nil
}

// Router implements the http.Handler interface for the Billing API
func Router(c client.Client) *mux.Router {
	r := mux.NewRouter()
	h := handlers{temporal: c}

	r.HandleFunc("/charge", h.handleCharge)

	return r
}

func (h *handlers) handleCharge(w http.ResponseWriter, r *http.Request) {
	var input ChargeInput

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		log.Println("Error: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	wf, err := h.temporal.ExecuteWorkflow(context.Background(),
		client.StartWorkflowOptions{
			TaskQueue: TaskQueue,
			ID:        input.Reference,
		},
		Charge,
		&input,
	)
	if err != nil {
		log.Println("Error: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var result ChargeResult
	err = wf.Get(r.Context(), &result)
	if err != nil {
		log.Println("Error: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(result)
	if err != nil {
		log.Println("Error: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}