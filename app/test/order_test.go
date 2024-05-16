package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/temporalio/orders-reference-app-go/app/billing"
	"github.com/temporalio/orders-reference-app-go/app/order"
	"github.com/temporalio/orders-reference-app-go/app/shipment"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"golang.org/x/sync/errgroup"
)

func postJSON(url string, input interface{}) (*http.Response, error) {
	jsonInput, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("unable to encode input: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(jsonInput))
	if err != nil {
		return nil, fmt.Errorf("unable to build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	return http.DefaultClient.Do(req)
}

func getJSON(url string, result interface{}) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to build request: %w", err)
	}

	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return r, err
	}
	defer r.Body.Close()

	if r.StatusCode >= 200 && r.StatusCode < 300 {
		if result == nil {
			return r, nil
		}

		err = json.NewDecoder(r.Body).Decode(result)
		return r, err
	}

	message, _ := io.ReadAll(r.Body)

	return r, fmt.Errorf("%s: %s", http.StatusText(r.StatusCode), message)
}

func Test_Order(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	s, err := testsuite.StartDevServer(ctx, testsuite.DevServerOptions{
		ClientOptions: &client.Options{},
		EnableUI:      true,
		ExtraArgs:     []string{"--dynamic-config-value", "system.forceSearchAttributesCacheRefreshOnRead=true"},
	})
	require.NoError(t, err)

	var (
		c client.Client
	)

	options := client.Options{
		HostPort:  s.FrontendHostPort(),
		Namespace: "default",
	}

	c, err = client.Dial(options)
	require.NoError(t, err)
	defer c.Close()

	err = shipment.EnsureValidTemporalEnv(ctx, options)
	require.NoError(t, err)

	billingAPI := httptest.NewServer(billing.Router(c))
	defer billingAPI.Close()
	orderAPI := httptest.NewServer(order.Router(c))
	defer orderAPI.Close()
	shipmentAPI := httptest.NewServer(shipment.Router(c))
	defer shipmentAPI.Close()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		return billing.RunWorker(ctx, c, billing.Config{FraudCheckURL: ""})
	})
	g.Go(func() error {
		return shipment.RunWorker(ctx, c)
	})
	g.Go(func() error {
		return order.RunWorker(ctx, c, order.Config{BillingURL: billingAPI.URL})
	})

	res, err := postJSON(orderAPI.URL+"/orders", &order.OrderInput{
		ID:         "order123",
		CustomerID: "customer123",
		Items: []*order.Item{
			{SKU: "Adidas Classic", Quantity: 1},
			{SKU: "Nike Air", Quantity: 2},
		},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, res.StatusCode)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var o order.OrderStatus
		res, err = getJSON(orderAPI.URL+"/orders/order123", &o)
		require.NoError(t, err)

		assert.Equal(c, "customerActionRequired", o.Status)
	}, 3*time.Second, 100*time.Millisecond)

	res, err = postJSON(orderAPI.URL+"/orders/order123/action", &order.CustomerActionSignal{
		Action: "amend",
	})
	require.Equal(t, http.StatusOK, res.StatusCode)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var o order.OrderStatus
		res, err := getJSON(orderAPI.URL+"/orders/order123", &o)
		require.NoError(t, err)

		require.Equal(c, http.StatusOK, res.StatusCode)
		assert.NotNil(c, o.Fulfillments[0].Shipment)
	}, 3*time.Second, 100*time.Millisecond)

	var o order.OrderStatus
	res, err = getJSON(orderAPI.URL+"/orders/order123", &o)
	require.NoError(t, err)

	for _, f := range o.Fulfillments {
		res, err := postJSON(shipmentAPI.URL+"/shipments/"+f.Shipment.ID+"/status", &shipment.ShipmentCarrierUpdateSignal{Status: "delivered"})
		require.Equal(t, http.StatusOK, res.StatusCode)
		require.NoError(t, err)
	}

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		var o order.OrderStatus
		res, err = getJSON(orderAPI.URL+"/orders/order123", &o)
		require.NoError(t, err)

		require.Equal(c, http.StatusOK, res.StatusCode)
		assert.Equal(c, "completed", o.Status)
	}, 3*time.Second, 100*time.Millisecond)

	cancel()

	err = g.Wait()
	require.NoError(t, err)

	err = s.Stop()
	require.NoError(t, err)
}
