package shipment

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/temporalio/orders-reference-app-go/app/internal/temporalutil"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
)

// TaskQueue is the default task queue for the Shipment system.
const TaskQueue = "shipments"

// StatusQuery is the name of the query to use to fetch a Shipment's status.
const StatusQuery = "status"

type handlers struct {
	temporal client.Client
}

// ShipmentStatus holds the status of a Shipment.
type ShipmentStatus struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updatedAt"`
	Items     []Item `json:"items"`
}

// ListShipmentEntry is an entry in the Shipment list.
type ListShipmentEntry struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// saConverter is a converter for deserializing search attributes.
var saConverter = converter.GetDefaultDataConverter()

// RunServer runs a Shipment API HTTP server on the given port.
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
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: Router(c),
	}

	fmt.Printf("Listening on http://127.0.0.1:%d\n", port)

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

// Router implements the http.Handler interface for the Shipment API
func Router(c client.Client) *mux.Router {
	r := mux.NewRouter()
	h := handlers{temporal: c}

	r.HandleFunc("/shipments", h.handleListShipments).Methods("GET")
	r.HandleFunc("/shipments/{id}", h.handleGetShipment).Methods("GET")
	r.HandleFunc("/shipments/{id}/status", h.handleUpdateShipmentStatus).Methods("POST")

	return r
}

func getStatusFromSearchAttributes(sa *common.SearchAttributes) (string, error) {
	if status, ok := sa.GetIndexedFields()["status"]; ok {
		var s string
		if err := saConverter.FromPayload(status, &s); err != nil {
			return "", err
		}
		return s, nil
	}
	return "unknown", nil
}

func (h *handlers) handleListShipments(w http.ResponseWriter, r *http.Request) {
	orders := []ListShipmentEntry{}
	var nextPageToken []byte

	for {
		resp, err := h.temporal.ListWorkflow(r.Context(), &workflowservice.ListWorkflowExecutionsRequest{
			PageSize:      10,
			NextPageToken: nextPageToken,
			Query:         "WorkflowType='Shipment' AND ExecutionStatus='Running'",
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, e := range resp.Executions {
			status, err := getStatusFromSearchAttributes(e.GetSearchAttributes())
			if err != nil {
				log.Default().Printf("unable to retrieve status search attribute for shipment: %v", err)
				status = "unknown"
			}

			orders = append(orders, ListShipmentEntry{ID: e.GetExecution().GetWorkflowId(), Status: status})
		}

		if len(resp.NextPageToken) == 0 {
			break
		}

		nextPageToken = resp.NextPageToken
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(orders); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *handlers) handleGetShipment(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	var status ShipmentStatus

	q, err := h.temporal.QueryWorkflow(r.Context(),
		vars["id"], "",
		StatusQuery,
	)
	if err != nil {
		switch err.(type) {
		case *serviceerror.NotFound:
			http.Error(w, "Shipment not found", http.StatusNotFound)
		default:
			log.Println("Error: ", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	if err := q.Get(&status); err != nil {
		log.Println("Error: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err = json.NewEncoder(w).Encode(status); err != nil {
		log.Println("Error: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (h *handlers) handleUpdateShipmentStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)

	var signal ShipmentCarrierUpdateSignal

	err := json.NewDecoder(r.Body).Decode(&signal)
	if err != nil {
		log.Println("Error: ", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = h.temporal.SignalWorkflow(context.Background(),
		vars["id"], "",
		ShipmentCarrierUpdateSignalName,
		signal,
	)
	if err != nil {
		switch err.(type) {
		case *serviceerror.NotFound:
			http.Error(w, "Shipment not found", http.StatusNotFound)
		default:
			log.Println("Error: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}
}
