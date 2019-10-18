package backend

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/3scale/3scale-go-client/client"
	"github.com/3scale/3scale-go-client/fake"
)

func TestCachedBackend_AuthRep(t *testing.T) {
	// we infer the cache key from the static response generated by rhe http client
	const cacheKey = "fake_example_"
	mockRequest := AuthRepRequest{
		ServiceID: "fake",
		Request: client.Request{
			Credentials: client.TokenAuth{
				Type:  "provider_key",
				Value: "any",
			},
		},
	}

	// create a client which returns a hierarchy in the following format: `<metric name="hits" children="example sample test" />`
	// hits metric has an initial current value of 1 and a max value of 4 for a minute
	// test_metric has an initial value of 0 and max of 6 for a week
	httpClient := NewTestClient(t, func(req *http.Request) *http.Response {
		return &http.Response{
			StatusCode: 200,
			Body:       ioutil.NopCloser(bytes.NewBufferString(fake.GetHierarchyEnabledResponse())),
			Header:     make(http.Header),
		}

	})

	inputs := []struct {
		name string
		// function to populate the cache with desired state
		cacheInit func() Cacheable
		// function to ensure correct behaviour internally and should evaluate to true in each case
		verifyBehaviour func(cacheable Cacheable) bool
		metrics         client.Metrics
		expectAuthz     bool
	}{
		{
			name:    "Test unaffected metrics returns success",
			metrics: client.Metrics{"orphan": 1},
			cacheInit: func() Cacheable {
				return NewLocalCache(nil, nil)
			},

			expectAuthz: true,
		},
		{
			name:    "Test direct hit on metric creates increment",
			metrics: client.Metrics{"hits": 1},
			cacheInit: func() Cacheable {
				return NewLocalCache(nil, nil)
			},
			verifyBehaviour: func(cacheable Cacheable) bool {
				cv, _ := cacheable.Get(cacheKey)
				return cv.LimitCounter["hits"][client.Minute].current == 2
			},
			expectAuthz: true,
		},
		{
			name:    "Test child metric increments parent metric",
			metrics: client.Metrics{"example": 1},
			cacheInit: func() Cacheable {
				return NewLocalCache(nil, nil)
			},
			verifyBehaviour: func(cacheable Cacheable) bool {
				cv, _ := cacheable.Get(cacheKey)
				return cv.LimitCounter["hits"][client.Minute].current == 2
			},
			expectAuthz: true,
		},
		{
			name:    "Test direct hit on metric causes unauthorized",
			metrics: client.Metrics{"hits": 4},
			cacheInit: func() Cacheable {
				return NewLocalCache(nil, nil)
			},
			expectAuthz: false,
		},
		{
			name:    "Test hit on child metric causes unauthorized",
			metrics: client.Metrics{"example": 4},
			cacheInit: func() Cacheable {
				return NewLocalCache(nil, nil)
			},
			expectAuthz: false,
		},
	}

	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			be, _ := client.NewBackend("http", "example", 80)
			threescaleClient := client.NewThreeScale(be, httpClient)

			cache := input.cacheInit()
			backend, _ := NewCachedBackend(cache, nil)

			request := mockRequest
			request.Params.Metrics = input.metrics

			resp, err := backend.AuthRep(request, threescaleClient)
			if err != nil {
				t.Errorf("unexpected error when calling AuthRep")
			}

			if resp.Success != input.expectAuthz {
				t.Errorf("unexpected result returned from AuthRep")
			}

			if input.verifyBehaviour != nil && !input.verifyBehaviour(cache) {
				t.Errorf("verification func fasiled to evaluate as expected")
			}
		})
	}

}
