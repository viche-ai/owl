package metrics

// modelPricing holds approximate per-1M-token prices (USD) for known models.
// Input and output prices are listed separately. These are estimates only.
var modelPricing = map[string][2]float64{
	// anthropic
	"claude-opus-4-6":           {15.00, 75.00},
	"claude-sonnet-4-6":         {3.00, 15.00},
	"claude-haiku-4-5":          {0.80, 4.00},
	"claude-haiku-4-5-20251001": {0.80, 4.00},
	// openai
	"gpt-4o":      {2.50, 10.00},
	"gpt-4o-mini": {0.15, 0.60},
	"gpt-4-turbo": {10.00, 30.00},
	"o3":          {10.00, 40.00},
	"o4-mini":     {1.10, 4.40},
	// google
	"gemini-2.5-pro":   {1.25, 10.00},
	"gemini-2.0-flash": {0.10, 0.40},
}

// EstimateCost returns the approximate USD cost for a run given the model ID
// and token counts. The model ID may be in "provider/model" form.
// Returns 0 if the model is not in the pricing table.
func EstimateCost(modelID string, tokensIn, tokensOut int) float64 {
	// strip provider prefix if present (e.g. "anthropic/claude-sonnet-4-6")
	name := modelID
	for i := 0; i < len(modelID); i++ {
		if modelID[i] == '/' {
			name = modelID[i+1:]
			break
		}
	}

	pricing, ok := modelPricing[name]
	if !ok {
		return 0
	}
	inputCost := float64(tokensIn) / 1_000_000 * pricing[0]
	outputCost := float64(tokensOut) / 1_000_000 * pricing[1]
	return inputCost + outputCost
}
