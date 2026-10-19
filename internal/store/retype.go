package store

import (
	"modelsdb/internal/dbcore"
)

// RederiveOpenRouterTypes re-derives model_type for every row OpenRouter created,
// from the modalities and description the row already holds, and returns how many
// rows it retyped.
//
// UpsertAPIModel types a row only while the feed still lists it under its stored
// name. A row the feed has renamed or dropped keeps the type it was last given,
// so a kind of model the app learned to type later (decisions, speech) would never
// reach it. Running this after the feed loop types every such row like the rest,
// with no migration: the next refresh corrects what is stored.
//
// A row that states neither a modality nor a type is left alone, because nothing
// says what it is. Only OpenRouter's own cell is written, so a type a repository
// stated or the reader researched is never touched.
func RederiveOpenRouterTypes() (int, error) {
	identities, err := ListModelIdentities()
	if err != nil {
		return 0, err
	}
	cells, err := AllCells()
	if err != nil {
		return 0, err
	}
	rows, err := dbcore.DB.Query(`SELECT v.model_name, v.value_text FROM cells v
		JOIN columns c ON c.id=v.column_id WHERE c.column_key='model_type' AND v.source=?`, string(SourceOpenRouter))
	if err != nil {
		return 0, err
	}
	statedType := map[string]string{}
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			rows.Close()
			return 0, err
		}
		statedType[name] = value
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	retyped := 0
	for _, identity := range identities {
		if identity.Source != string(SourceOpenRouter) {
			continue
		}
		held := cells[identity.Name]
		modalities := dbcore.ParseArr(held["output_modalities"])
		if len(modalities) == 0 && statedType[identity.Name] == "" {
			continue
		}
		derived := deriveType(modalities, held["description"])
		if statedType[identity.Name] == derived {
			continue
		}
		if err := WriteCells(SourceOpenRouter, identity.Name, map[string]string{"model_type": derived}); err != nil {
			return retyped, err
		}
		retyped++
	}
	return retyped, nil
}
