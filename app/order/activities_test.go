package order_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/orders-reference-app-go/app/order"
	"go.temporal.io/sdk/testsuite"
)

func TestFulfillOrderZeroItems(t *testing.T) {
	testSuite := testsuite.WorkflowTestSuite{}

	var a *order.Activities

	env := testSuite.NewTestActivityEnvironment()
	env.RegisterActivity(a.FulfillOrder)

	input := order.FulfillOrderInput{
		Items: []order.Item{},
	}

	future, err := env.ExecuteActivity(a.FulfillOrder, input)
	require.NoError(t, err)

	var result order.FulfillOrderResult
	require.NoError(t, future.Get(&result))

	expected := order.FulfillOrderResult{}

	require.Equal(t, expected, result)
}

func TestFulfillOrderOneItem(t *testing.T) {
	testSuite := testsuite.WorkflowTestSuite{}

	var a *order.Activities

	env := testSuite.NewTestActivityEnvironment()
	env.RegisterActivity(a.FulfillOrder)

	input := order.FulfillOrderInput{
		Items: []order.Item{
			{SKU: "Hiking Boots", Quantity: 2},
		},
	}

	future, err := env.ExecuteActivity(a.FulfillOrder, input)
	require.NoError(t, err)

	var result order.FulfillOrderResult
	require.NoError(t, future.Get(&result))

	expected := order.FulfillOrderResult{
		Fulfillments: []order.Fulfillment{
			{
				Location: "Warehouse A",
				Items: []order.Item{
					{SKU: "Hiking Boots", Quantity: 2},
				},
			},
		},
	}

	require.Equal(t, expected, result)
}

func TestFulfillOrderTwoItems(t *testing.T) {
	testSuite := testsuite.WorkflowTestSuite{}

	var a *order.Activities

	env := testSuite.NewTestActivityEnvironment()
	env.RegisterActivity(a.FulfillOrder)

	input := order.FulfillOrderInput{
		Items: []order.Item{
			{SKU: "Hiking Boots", Quantity: 2},
			{SKU: "Tennis Shoes", Quantity: 1},
		},
	}

	future, err := env.ExecuteActivity(a.FulfillOrder, input)
	require.NoError(t, err)

	var result order.FulfillOrderResult
	require.NoError(t, future.Get(&result))

	expected := order.FulfillOrderResult{
		Fulfillments: []order.Fulfillment{
			{
				Location: "Warehouse A",
				Items: []order.Item{
					{SKU: "Hiking Boots", Quantity: 2},
				},
			},
			{
				Location: "Warehouse B",
				Items: []order.Item{
					{SKU: "Tennis Shoes", Quantity: 1},
				},
			},
		},
	}

	require.Equal(t, expected, result)
}
