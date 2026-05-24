package providers

import "testing"

func TestAzureSourceHash_Deterministic(t *testing.T) {
	a := azureSourceHash("2026-05-20T00:00:00Z", "subscriptions/X", "/res", "GPT-4o Input", "Azure OpenAI - GPT-4o", "1K Tokens")
	b := azureSourceHash("2026-05-20T00:00:00Z", "subscriptions/X", "/res", "GPT-4o Input", "Azure OpenAI - GPT-4o", "1K Tokens")
	if a != b {
		t.Fatalf("same inputs -> different hashes: %s vs %s", a, b)
	}
	cases := []struct {
		name                                 string
		date, scope, resID, meter, sku, unit string
	}{
		{"different date", "2026-05-21T00:00:00Z", "subscriptions/X", "/res", "GPT-4o Input", "Azure OpenAI - GPT-4o", "1K Tokens"},
		{"different scope", "2026-05-20T00:00:00Z", "subscriptions/Y", "/res", "GPT-4o Input", "Azure OpenAI - GPT-4o", "1K Tokens"},
		{"different resourceId", "2026-05-20T00:00:00Z", "subscriptions/X", "/res2", "GPT-4o Input", "Azure OpenAI - GPT-4o", "1K Tokens"},
		{"different meter", "2026-05-20T00:00:00Z", "subscriptions/X", "/res", "GPT-4o Output", "Azure OpenAI - GPT-4o", "1K Tokens"},
		{"different sku", "2026-05-20T00:00:00Z", "subscriptions/X", "/res", "GPT-4o Input", "Azure OpenAI - GPT-3.5", "1K Tokens"},
		{"different unit", "2026-05-20T00:00:00Z", "subscriptions/X", "/res", "GPT-4o Input", "Azure OpenAI - GPT-4o", "Hour"},
	}
	for _, c := range cases {
		other := azureSourceHash(c.date, c.scope, c.resID, c.meter, c.sku, c.unit)
		if other == a {
			t.Errorf("%s should differ but matched", c.name)
		}
	}
}

func TestAzure_ModelExtraction(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Azure OpenAI - GPT-4o", "gpt-4o"},
		{"GPT-3.5-Turbo Input Tokens", "gpt-3.5-turbo"},
		{"text-embedding-3-small", "text-embedding-3-small"},
		{"text-embedding-ada-002", "text-embedding-ada-002"},
		{"GPT-4o-mini Output", "gpt-4o-mini"},
		{"o1-mini Output", "o1-mini"},
		{"whisper-1", "whisper-1"},
		{"DALL-E 3", ""},
		{"Random non-OpenAI meter", ""},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := extractAzureModel(c.in)
			if got != c.want {
				t.Errorf("extractAzureModel(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestAzure_ProjectIDFallback(t *testing.T) {
	cases := []struct {
		name          string
		resourceID    string
		resourceGroup string
		wantProjectID string
	}{
		{"resource id present", "/subscriptions/X/rg/myacct", "rg", "/subscriptions/X/rg/myacct"},
		{"falls back to rg", "", "my-rg", "my-rg"},
		{"falls back to scope", "", "", "subscriptions/" + azureTestSubscription},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := azureCostRow{
				Date:          "2026-05-20T00:00:00Z",
				Cost:          1.0,
				Currency:      "USD",
				ResourceID:    c.resourceID,
				ResourceGroup: c.resourceGroup,
				ServiceName:   "Azure OpenAI",
				Meter:         "GPT-4o Input",
				MeterCategory: "Azure OpenAI",
			}
			r := mapAzureCostRow(row, "subscriptions/"+azureTestSubscription)
			if r.ProjectID != c.wantProjectID {
				t.Errorf("ProjectID = %q, want %q", r.ProjectID, c.wantProjectID)
			}
		})
	}
}

func TestAzure_SortStable(t *testing.T) {
	records := []NormalizedCostRecord{
		{RecordedAt: "2026-05-21T00:00:00Z", ProjectID: "b", SKU: "z", Model: "gpt-4o", UnitType: "1K Tokens"},
		{RecordedAt: "2026-05-20T00:00:00Z", ProjectID: "a", SKU: "x", Model: "gpt-4o-mini", UnitType: "1K Tokens"},
		{RecordedAt: "2026-05-20T00:00:00Z", ProjectID: "a", SKU: "y", Model: "gpt-4o", UnitType: "1K Tokens"},
	}
	sortAzureRecords(records)
	if records[0].SKU != "x" || records[1].SKU != "y" || records[2].SKU != "z" {
		t.Errorf("unexpected sort: %+v", records)
	}
}
