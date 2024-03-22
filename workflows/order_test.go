package workflows_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/temporalio/orders-reference-app-go/internal/shipmentapi"
	"github.com/temporalio/orders-reference-app-go/pkg/ordersapi"
	"github.com/temporalio/orders-reference-app-go/workflows"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

func TestOrderWorkflow(t *testing.T) {
	s := testsuite.WorkflowTestSuite{}
	env := s.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(workflows.Order)

	orderInput := ordersapi.OrderInput{
		Items: []ordersapi.Item{
			{SKU: "test1", Quantity: 1},
			{SKU: "test2", Quantity: 3},
		},
	}

	env.OnWorkflow(workflows.Shipment, mock.Anything, mock.Anything).Return(func(ctx workflow.Context, input shipmentapi.ShipmentInput) (shipmentapi.ShipmentResult, error) {
		return shipmentapi.ShipmentResult{}, nil
	})

	env.ExecuteWorkflow(
		workflows.Order,
		orderInput,
	)

	var result ordersapi.OrderResult
	err := env.GetWorkflowResult(&result)
	assert.NoError(t, err)
}