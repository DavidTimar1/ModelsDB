package store

import (
	"reflect"
	"testing"
)

func TestHasErrorPrice(t *testing.T) {
	if !hasErrorPrice(map[string]interface{}{"prompt": -1}) {
		t.Errorf("negative price should be an error price")
	}
	if !hasErrorPrice(map[string]interface{}{"image": -1000000}) {
		t.Errorf("negative sentinel should be an error price")
	}
	if hasErrorPrice(map[string]interface{}{"prompt": "0.5", "completion": "1"}) {
		t.Errorf("positive prices should not be error prices")
	}
	if hasErrorPrice(map[string]interface{}{"prompt": nil}) {
		t.Errorf("nil price should be ignored")
	}
}

func TestToStrList(t *testing.T) {
	if got := toStrList([]interface{}{"a", 1}); !reflect.DeepEqual(got, []string{"a", "1"}) {
		t.Errorf("toStrList([]interface{}) = %v", got)
	}
	if got := toStrList([]string{"x"}); !reflect.DeepEqual(got, []string{"x"}) {
		t.Errorf("toStrList([]string) = %v", got)
	}
	if got := toStrList(123); got != nil {
		t.Errorf("toStrList(unsupported) = %v, want nil", got)
	}
}

func TestPricingOf(t *testing.T) {
	norm := map[string]interface{}{"endpoint": map[string]interface{}{
		"pricing": map[string]interface{}{"prompt": "0.1"},
	}}
	if got := pricingOf(norm); got["prompt"] != "0.1" {
		t.Errorf("pricingOf should return the nested pricing map, got %v", got)
	}
	if got := pricingOf(map[string]interface{}{}); len(got) != 0 {
		t.Errorf("pricingOf without endpoint should be empty, got %v", got)
	}
}

func TestPriceStr(t *testing.T) {
	pr := map[string]interface{}{"prompt": 0.5, "nil": nil}
	if priceStr(pr, "prompt") != "0.5" {
		t.Errorf("priceStr present")
	}
	if priceStr(pr, "nil") != "" {
		t.Errorf("priceStr nil should be ''")
	}
	if priceStr(pr, "absent") != "" {
		t.Errorf("priceStr absent should be ''")
	}
}

func TestDeriveType(t *testing.T) {
	cases := []struct {
		out  []string
		want string
	}{
		{[]string{"image"}, "image"},
		{[]string{"video"}, "video"},
		{[]string{"audio"}, "tts"},
		{[]string{"text"}, "chat"},
		{nil, "chat"},
	}
	for _, c := range cases {
		if got := deriveType(c.out); got != c.want {
			t.Errorf("deriveType(%v) = %q, want %q", c.out, got, c.want)
		}
	}
}

func TestJSONArrStr(t *testing.T) {
	if jsonArrStr(nil) != "" {
		t.Errorf("empty list should be ''")
	}
	if got := jsonArrStr([]string{"a", "b"}); got != `["a","b"]` {
		t.Errorf("jsonArrStr = %q", got)
	}
}
