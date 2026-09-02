package db

import (
	"encoding/json"
	"fmt"
	"time"
)

// DatabaseBackup matches the exact 9router exportDb/importDb JSON schema.
type DatabaseBackup struct {
	Settings            *SettingsData          `json:"settings,omitempty"`
	ProviderConnections []map[string]any       `json:"providerConnections"`
	ProviderNodes       []map[string]any       `json:"providerNodes"`
	ProxyPools          []map[string]any       `json:"proxyPools"`
	APIKeys             []map[string]any       `json:"apiKeys"`
	Combos              []map[string]any       `json:"combos"`
	ModelAliases        map[string]any         `json:"modelAliases"`
	CustomModels        []CustomModelEntry     `json:"customModels"`
	ProviderPrefixes    map[string]string      `json:"providerPrefixes,omitempty"`
	MitmAlias           map[string]any         `json:"mitmAlias,omitempty"`
	Pricing             map[string]any         `json:"pricing,omitempty"`
}

// ExportDB exports the complete SQLite database matching 9router's backup payload.
func (r *Repo) ExportDB() (*DatabaseBackup, error) {
	out := &DatabaseBackup{
		ProviderConnections: make([]map[string]any, 0),
		ProviderNodes:       make([]map[string]any, 0),
		ProxyPools:          make([]map[string]any, 0),
		APIKeys:             make([]map[string]any, 0),
		Combos:              make([]map[string]any, 0),
		ModelAliases:        make(map[string]any),
		CustomModels:        make([]CustomModelEntry, 0),
		ProviderPrefixes:    make(map[string]string),
		MitmAlias:           make(map[string]any),
		Pricing:             make(map[string]any),
	}

	// 1. Settings
	if s, err := r.GetSettings(); err == nil && s != nil {
		out.Settings = s
	}

	// 2. Provider Connections
	if rows, err := r.db.Query("SELECT id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt FROM providerConnections"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, provider, authType, data, createdAt, updatedAt string
			var name, email *string
			var priority *int
			var isActive int
			if err := rows.Scan(&id, &provider, &authType, &name, &email, &priority, &isActive, &data, &createdAt, &updatedAt); err == nil {
				entry := make(map[string]any)
				if data != "" {
					_ = json.Unmarshal([]byte(data), &entry)
				}
				entry["id"] = id
				entry["provider"] = provider
				entry["authType"] = authType
				if name != nil {
					entry["name"] = *name
				}
				if email != nil {
					entry["email"] = *email
				}
				if priority != nil {
					entry["priority"] = *priority
				}
				entry["isActive"] = isActive == 1
				entry["createdAt"] = createdAt
				entry["updatedAt"] = updatedAt
				out.ProviderConnections = append(out.ProviderConnections, entry)
			}
		}
	}

	// 3. Provider Nodes
	if rows, err := r.db.Query("SELECT id, type, name, data, createdAt, updatedAt FROM providerNodes"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, nodeType, name, data, createdAt, updatedAt string
			if err := rows.Scan(&id, &nodeType, &name, &data, &createdAt, &updatedAt); err == nil {
				entry := make(map[string]any)
				if data != "" {
					_ = json.Unmarshal([]byte(data), &entry)
				}
				entry["id"] = id
				entry["type"] = nodeType
				entry["name"] = name
				entry["createdAt"] = createdAt
				entry["updatedAt"] = updatedAt
				out.ProviderNodes = append(out.ProviderNodes, entry)
			}
		}
	}

	// 4. Proxy Pools
	if rows, err := r.db.Query("SELECT id, isActive, testStatus, data, createdAt, updatedAt FROM proxyPools"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, testStatus, data, createdAt, updatedAt string
			var isActive int
			if err := rows.Scan(&id, &isActive, &testStatus, &data, &createdAt, &updatedAt); err == nil {
				entry := make(map[string]any)
				if data != "" {
					_ = json.Unmarshal([]byte(data), &entry)
				}
				entry["id"] = id
				entry["isActive"] = isActive == 1
				entry["testStatus"] = testStatus
				entry["createdAt"] = createdAt
				entry["updatedAt"] = updatedAt
				out.ProxyPools = append(out.ProxyPools, entry)
			}
		}
	}

	// 5. API Keys
	if rows, err := r.db.Query("SELECT id, key, name, restrictions, isActive, createdAt FROM apiKeys"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, key, createdAt string
			var name, restrictions *string
			var isActive int
			if err := rows.Scan(&id, &key, &name, &restrictions, &isActive, &createdAt); err == nil {
				entry := map[string]any{
					"id":        id,
					"key":       key,
					"isActive":  isActive == 1,
					"createdAt": createdAt,
				}
				if name != nil {
					entry["name"] = *name
				}
				if restrictions != nil && *restrictions != "" {
					var rObj any
					if json.Unmarshal([]byte(*restrictions), &rObj) == nil {
						entry["restrictions"] = rObj
					}
				}
				out.APIKeys = append(out.APIKeys, entry)
			}
		}
	}

	// 6. Combos
	if rows, err := r.db.Query("SELECT id, name, strategy, models, createdAt, updatedAt FROM combos"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var id, name, strategy, models, createdAt, updatedAt string
			if err := rows.Scan(&id, &name, &strategy, &models, &createdAt, &updatedAt); err == nil {
				var modelsList any
				if json.Unmarshal([]byte(models), &modelsList) != nil {
					modelsList = []string{}
				}
				out.Combos = append(out.Combos, map[string]any{
					"id":        id,
					"name":      name,
					"strategy":  strategy,
					"models":    modelsList,
					"createdAt": createdAt,
					"updatedAt": updatedAt,
				})
			}
		}
	}

	// 7. KV table entries
	if rows, err := r.db.Query("SELECT scope, key, value FROM kv"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var scope, k, v string
			if err := rows.Scan(&scope, &k, &v); err == nil {
				var parsed any
				if json.Unmarshal([]byte(v), &parsed) != nil {
					parsed = v
				}
				switch scope {
				case "modelAliases":
					out.ModelAliases[k] = parsed
				case "customModels":
					if cm, ok := parsed.(map[string]any); ok {
						pAlias, _ := cm["providerAlias"].(string)
						mID, _ := cm["id"].(string)
						mType, _ := cm["type"].(string)
						mName, _ := cm["name"].(string)
						out.CustomModels = append(out.CustomModels, CustomModelEntry{
							ProviderAlias: pAlias,
							ID:            mID,
							Type:          mType,
							Name:          mName,
						})
					}
				case "providerPrefixes":
					if strVal, ok := parsed.(string); ok {
						out.ProviderPrefixes[k] = strVal
					}
				case "mitmAlias":
					out.MitmAlias[k] = parsed
				case "pricing":
					out.Pricing[k] = parsed
				}
			}
		}
	}

	return out, nil
}

// ImportDB atomically restores a complete 9router/Zyrouter backup JSON into SQLite.
func (r *Repo) ImportDB(backup *DatabaseBackup) error {
	if backup == nil {
		return fmt.Errorf("empty backup payload")
	}

	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Clean existing tables (except migration meta)
	tables := []string{"providerConnections", "providerNodes", "proxyPools", "apiKeys", "combos", "kv"}
	for _, t := range tables {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			return fmt.Errorf("wipe table %s: %w", t, err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// 2. Settings
	if backup.Settings != nil {
		b, _ := json.Marshal(backup.Settings)
		_, err := tx.Exec(`INSERT INTO settings(id, data) VALUES(1, ?)
			ON CONFLICT(id) DO UPDATE SET data = excluded.data`, string(b))
		if err != nil {
			return fmt.Errorf("restore settings: %w", err)
		}
	}

	// 3. Provider Connections
	for _, c := range backup.ProviderConnections {
		id, _ := c["id"].(string)
		if id == "" {
			continue
		}
		provider, _ := c["provider"].(string)
		authType, _ := c["authType"].(string)
		if authType == "" {
			authType = "apikey"
		}
		var name, email *string
		if n, ok := c["name"].(string); ok && n != "" {
			name = &n
		}
		if em, ok := c["email"].(string); ok && em != "" {
			email = &em
		}
		var priority *int
		if p, ok := c["priority"].(float64); ok {
			pInt := int(p)
			priority = &pInt
		}
		isActive := 1
		if ia, ok := c["isActive"].(bool); ok && !ia {
			isActive = 0
		}
		cCreatedAt, _ := c["createdAt"].(string)
		if cCreatedAt == "" {
			cCreatedAt = now
		}
		cUpdatedAt, _ := c["updatedAt"].(string)
		if cUpdatedAt == "" {
			cUpdatedAt = now
		}

		delete(c, "id")
		delete(c, "provider")
		delete(c, "authType")
		delete(c, "name")
		delete(c, "email")
		delete(c, "priority")
		delete(c, "isActive")
		delete(c, "createdAt")
		delete(c, "updatedAt")

		dataBytes, _ := json.Marshal(c)
		_, err := tx.Exec(`INSERT INTO providerConnections (id, provider, authType, name, email, priority, isActive, data, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, provider, authType, name, email, priority, isActive, string(dataBytes), cCreatedAt, cUpdatedAt)
		if err != nil {
			return fmt.Errorf("restore providerConnection %s: %w", id, err)
		}
	}

	// 4. Provider Nodes
	for _, n := range backup.ProviderNodes {
		id, _ := n["id"].(string)
		if id == "" {
			continue
		}
		nodeType, _ := n["type"].(string)
		name, _ := n["name"].(string)
		nCreatedAt, _ := n["createdAt"].(string)
		if nCreatedAt == "" {
			nCreatedAt = now
		}
		nUpdatedAt, _ := n["updatedAt"].(string)
		if nUpdatedAt == "" {
			nUpdatedAt = now
		}
		delete(n, "id")
		delete(n, "type")
		delete(n, "name")
		delete(n, "createdAt")
		delete(n, "updatedAt")
		dataBytes, _ := json.Marshal(n)
		_, err := tx.Exec(`INSERT INTO providerNodes (id, type, name, data, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?, ?)`, id, nodeType, name, string(dataBytes), nCreatedAt, nUpdatedAt)
		if err != nil {
			return fmt.Errorf("restore providerNode %s: %w", id, err)
		}
	}

	// 5. Proxy Pools
	for _, p := range backup.ProxyPools {
		id, _ := p["id"].(string)
		if id == "" {
			continue
		}
		isActive := 1
		if ia, ok := p["isActive"].(bool); ok && !ia {
			isActive = 0
		}
		testStatus, _ := p["testStatus"].(string)
		if testStatus == "" {
			testStatus = "unknown"
		}
		pCreatedAt, _ := p["createdAt"].(string)
		if pCreatedAt == "" {
			pCreatedAt = now
		}
		pUpdatedAt, _ := p["updatedAt"].(string)
		if pUpdatedAt == "" {
			pUpdatedAt = now
		}
		delete(p, "id")
		delete(p, "isActive")
		delete(p, "testStatus")
		delete(p, "createdAt")
		delete(p, "updatedAt")
		dataBytes, _ := json.Marshal(p)
		_, err := tx.Exec(`INSERT INTO proxyPools (id, isActive, testStatus, data, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?, ?)`, id, isActive, testStatus, string(dataBytes), pCreatedAt, pUpdatedAt)
		if err != nil {
			return fmt.Errorf("restore proxyPool %s: %w", id, err)
		}
	}

	// 6. API Keys
	for _, k := range backup.APIKeys {
		id, _ := k["id"].(string)
		key, _ := k["key"].(string)
		if id == "" || key == "" {
			continue
		}
		var name *string
		if n, ok := k["name"].(string); ok && n != "" {
			name = &n
		}
		var restrictions *string
		if rObj, ok := k["restrictions"]; ok {
			rBytes, _ := json.Marshal(rObj)
			rStr := string(rBytes)
			restrictions = &rStr
		}
		isActive := 1
		if ia, ok := k["isActive"].(bool); ok && !ia {
			isActive = 0
		}
		kCreatedAt, _ := k["createdAt"].(string)
		if kCreatedAt == "" {
			kCreatedAt = now
		}
		_, err := tx.Exec(`INSERT INTO apiKeys (id, key, name, restrictions, isActive, createdAt)
			VALUES (?, ?, ?, ?, ?, ?)`, id, key, name, restrictions, isActive, kCreatedAt)
		if err != nil {
			return fmt.Errorf("restore apiKey %s: %w", id, err)
		}
	}

	// 7. Combos
	for _, c := range backup.Combos {
		id, _ := c["id"].(string)
		name, _ := c["name"].(string)
		strategy, _ := c["strategy"].(string)
		if strategy == "" {
			strategy, _ = c["kind"].(string)
		}
		if strategy == "" {
			strategy = "fallback"
		}
		modelsBytes, _ := json.Marshal(c["models"])
		cCreatedAt, _ := c["createdAt"].(string)
		if cCreatedAt == "" {
			cCreatedAt = now
		}
		cUpdatedAt, _ := c["updatedAt"].(string)
		if cUpdatedAt == "" {
			cUpdatedAt = now
		}
		_, err := tx.Exec(`INSERT INTO combos (id, name, strategy, models, createdAt, updatedAt)
			VALUES (?, ?, ?, ?, ?, ?)`, id, name, strategy, string(modelsBytes), cCreatedAt, cUpdatedAt)
		if err != nil {
			return fmt.Errorf("restore combo %s: %w", id, err)
		}
	}

	// 8. KV mappings
	for alias, target := range backup.ModelAliases {
		tBytes, _ := json.Marshal(target)
		_, _ = tx.Exec(`INSERT INTO kv (scope, key, value) VALUES ('modelAliases', ?, ?)`, alias, string(tBytes))
	}
	for _, cm := range backup.CustomModels {
		k := fmt.Sprintf("%s|%s|%s", cm.ProviderAlias, cm.ID, cm.Type)
		vBytes, _ := json.Marshal(cm)
		_, _ = tx.Exec(`INSERT INTO kv (scope, key, value) VALUES ('customModels', ?, ?)`, k, string(vBytes))
	}
	for prov, pref := range backup.ProviderPrefixes {
		vBytes, _ := json.Marshal(pref)
		_, _ = tx.Exec(`INSERT INTO kv (scope, key, value) VALUES ('providerPrefixes', ?, ?)`, prov, string(vBytes))
	}
	for tool, mitmMap := range backup.MitmAlias {
		vBytes, _ := json.Marshal(mitmMap)
		_, _ = tx.Exec(`INSERT INTO kv (scope, key, value) VALUES ('mitmAlias', ?, ?)`, tool, string(vBytes))
	}
	for prov, priceMap := range backup.Pricing {
		vBytes, _ := json.Marshal(priceMap)
		_, _ = tx.Exec(`INSERT INTO kv (scope, key, value) VALUES ('pricing', ?, ?)`, prov, string(vBytes))
	}

	return tx.Commit()
}
