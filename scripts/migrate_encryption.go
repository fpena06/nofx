package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	"nofx/crypto"

	_ "modernc.org/sqlite"
)

func main() {
	log.Println("🔄 Starting database migration to encrypted format...")

	// 1. 檢查數據庫檔案
	dbPath := "config.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatalf("❌ Database file does not exist: %s", dbPath)
	}

	// 2. 備份數據庫
	backupPath := fmt.Sprintf("%s.pre_encryption_backup", dbPath)
	log.Printf("📦 Backing up database to: %s", backupPath)

	input, err := os.ReadFile(dbPath)
	if err != nil {
		log.Fatalf("❌ Failed to read database: %v", err)
	}

	if err := os.WriteFile(backupPath, input, 0600); err != nil {
		log.Fatalf("❌ Backup failed: %v", err)
	}

	// 3. 打開數據庫
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("❌ Failed to open database: %v", err)
	}
	defer db.Close()

	// 4. 初始化加密管理器
	em, err := crypto.GetEncryptionManager()
	if err != nil {
		log.Fatalf("❌ Failed to initialize encryption manager: %v", err)
	}

	// 5. 遷移交易所配置
	if err := migrateExchanges(db, em); err != nil {
		log.Fatalf("❌ Failed to migrate exchange configurations: %v", err)
	}

	// 6. 遷移 AI 模型配置
	if err := migrateAIModels(db, em); err != nil {
		log.Fatalf("❌ Failed to migrate AI model configurations: %v", err)
	}

	log.Println("✅ Data migration completed!")
	log.Printf("📝 Original data backup located at: %s", backupPath)
	log.Println("⚠️  Please verify system functionality before manually deleting the backup file")
}

// migrateExchanges 遷移交易所配置
func migrateExchanges(db *sql.DB, em *crypto.EncryptionManager) error {
	log.Println("🔄 Migrating exchange configurations...")

	// 查詢所有未加密的記錄（假設加密數據都包含 '==' Base64 特徵）
	rows, err := db.Query(`
		SELECT user_id, id, api_key, secret_key,
		       COALESCE(hyperliquid_private_key, ''),
		       COALESCE(aster_private_key, '')
		FROM exchanges
		WHERE (api_key != '' AND api_key NOT LIKE '%==%')
		   OR (secret_key != '' AND secret_key NOT LIKE '%==%')
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	count := 0
	for rows.Next() {
		var userID, exchangeID, apiKey, secretKey, hlPrivateKey, asterPrivateKey string
		if err := rows.Scan(&userID, &exchangeID, &apiKey, &secretKey, &hlPrivateKey, &asterPrivateKey); err != nil {
			return err
		}

		// 加密每個字段
		encAPIKey, err := em.EncryptForDatabase(apiKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt API Key: %w", err)
		}

		encSecretKey, err := em.EncryptForDatabase(secretKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt Secret Key: %w", err)
		}

		encHLPrivateKey := ""
		if hlPrivateKey != "" {
			encHLPrivateKey, err = em.EncryptForDatabase(hlPrivateKey)
			if err != nil {
				return fmt.Errorf("failed to encrypt Hyperliquid Private Key: %w", err)
			}
		}

		encAsterPrivateKey := ""
		if asterPrivateKey != "" {
			encAsterPrivateKey, err = em.EncryptForDatabase(asterPrivateKey)
			if err != nil {
				return fmt.Errorf("failed to encrypt Aster Private Key: %w", err)
			}
		}

		// 更新數據庫
		_, err = tx.Exec(`
			UPDATE exchanges
			SET api_key = ?, secret_key = ?,
			    hyperliquid_private_key = ?, aster_private_key = ?
			WHERE user_id = ? AND id = ?
		`, encAPIKey, encSecretKey, encHLPrivateKey, encAsterPrivateKey, userID, exchangeID)

		if err != nil {
			return fmt.Errorf("failed to update database: %w", err)
		}

		log.Printf("  ✓ Encrypted: [%s] %s", userID, exchangeID)
		count++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("✅ Migrated %d exchange configurations", count)
	return nil
}

// migrateAIModels 遷移 AI 模型配置
func migrateAIModels(db *sql.DB, em *crypto.EncryptionManager) error {
	log.Println("🔄 Migrating AI model configurations...")

	rows, err := db.Query(`
		SELECT user_id, id, api_key
		FROM ai_models
		WHERE api_key != '' AND api_key NOT LIKE '%==%'
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	count := 0
	for rows.Next() {
		var userID, modelID, apiKey string
		if err := rows.Scan(&userID, &modelID, &apiKey); err != nil {
			return err
		}

		encAPIKey, err := em.EncryptForDatabase(apiKey)
		if err != nil {
			return fmt.Errorf("failed to encrypt API Key: %w", err)
		}

		_, err = tx.Exec(`
			UPDATE ai_models SET api_key = ? WHERE user_id = ? AND id = ?
		`, encAPIKey, userID, modelID)

		if err != nil {
			return fmt.Errorf("failed to update database: %w", err)
		}

		log.Printf("  ✓ Encrypted: [%s] %s", userID, modelID)
		count++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("✅ Migrated %d AI model configurations", count)
	return nil
}
