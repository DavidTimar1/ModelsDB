// Package app handles process logic and functionalities.
//
// processModels reads the single SQLite table and produces the row shape the
// frontend table expects (modalities joined to "a, b", pricing flattened,
// curated fields included). No cross-file merging anymore.
package app

import (
	"encoding/json"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"strings"
)

func joinMods(r map[string]interface{}, key string) string {
	return strings.Join(dbcore.ParseArr(shared.GetStr(r, key)), ", ")
}

func processModels() ([]map[string]interface{}, error) {
	rows, err := dbcore.GetAllModels()
	if err != nil {
		return nil, err
	}

	merged := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		name := shared.GetStr(r, "name")
		if name == "" || strings.Contains(strings.ToLower(name), "(free)") {
			continue
		}

		var raw map[string]interface{}
		if rj := shared.GetStr(r, "raw_json"); rj != "" {
			_ = json.Unmarshal([]byte(rj), &raw)
		}

		row := map[string]interface{}{
			"id":                 shared.GetStr(r, "id"),
			"slug":               shared.GetStr(r, "slug"),
			"name":               name,
			"description":        shared.GetStr(r, "description"),
			"created_at":         shared.GetStr(r, "created_at"),
			"hf_slug":            shared.GetStr(r, "hf_slug"),
			"context_length":     r["context_length"],
			"input_modalities":   joinMods(r, "input_modalities"),
			"output_modalities":  joinMods(r, "output_modalities"),
			"hidden":             shared.GetNum(r, "hidden") == 1,
			"supports_reasoning": shared.GetNum(r, "supports_reasoning") == 1,
			"zdr":                shared.GetNum(r, "zdr") == 1,

			"prompt":             shared.GetStr(r, "price_prompt"),
			"completion":         shared.GetStr(r, "price_completion"),
			"image":              shared.GetStr(r, "price_image"),
			"audio":              shared.GetStr(r, "price_audio"),
			"internal_reasoning": shared.GetStr(r, "price_internal_reasoning"),
			"price_display":      shared.GetStr(r, "price_display"),
			"pricing_url":        shared.GetStr(r, "pricing_url"),
			"measurement":        shared.GetStr(r, "measurement"),
			"pricing_note":       shared.GetStr(r, "pricing_note"),

			"notes":             shared.GetStr(r, "notes"),
			"speed":             shared.GetStr(r, "speed"),
			"rating":            shared.GetNum(r, "rating"),
			"favorite":          shared.GetNum(r, "favorite"),
			"ocr_quality":       shared.GetStr(r, "ocr_quality"),
			"tool":              shared.GetStr(r, "tool"),
			"moe":               shared.GetStr(r, "moe"),
			"parameters":        r["parameters"],
			"active_parameters": r["active_parameters"],
			"disk_size_gb":      r["disk_size_gb"],

			// New, additive fields (frontend may ignore them safely).
			"model_type":  shared.GetStr(r, "model_type"),
			"source":      shared.GetStr(r, "source"),
			"collections": dbcore.ParseArr(shared.GetStr(r, "collections")),

			"_raw": raw,
		}

		if row["tool"] == "" {
			row["tool"] = "no"
		}
		if row["moe"] == "" {
			row["moe"] = "unknown"
		}

		merged = append(merged, row)
	}

	return merged, nil
}
