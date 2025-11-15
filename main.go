package main

import (
	"encoding/json"
	"fmt"
	"log"
	"nofx/api"
	"nofx/auth"
	"nofx/config"
	"nofx/crypto"
	"nofx/manager"
	"nofx/market"
	"nofx/pool"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
)

// ConfigFile 配置文件结构，只包含需要同步到数据库的字段
// TODO 现在与config.Config相同，未来会被替换， 现在为了兼容性不得不保留当前文件
type ConfigFile struct {
	BetaMode           bool                  `json:"beta_mode"`
	APIServerPort      int                   `json:"api_server_port"`
	UseDefaultCoins    bool                  `json:"use_default_coins"`
	DefaultCoins       []string              `json:"default_coins"`
	CoinPoolAPIURL     string                `json:"coin_pool_api_url"`
	OITopAPIURL        string                `json:"oi_top_api_url"`
	MaxDailyLoss       float64               `json:"max_daily_loss"`
	MaxDrawdown        float64               `json:"max_drawdown"`
	StopTradingMinutes int                   `json:"stop_trading_minutes"`
	Leverage           config.LeverageConfig `json:"leverage"`
	JWTSecret          string                `json:"jwt_secret"`
	DataKLineTime      string                `json:"data_k_line_time"`
	Log                *config.LogConfig     `json:"log"` // 日志配置
}

// loadConfigFile 读取并解析config.json文件
func loadConfigFile() (*ConfigFile, error) {
	// 检查config.json是否存在
	if _, err := os.Stat("config.json"); os.IsNotExist(err) {
		log.Printf("📄 config.json does not exist, using default configuration")
		return &ConfigFile{}, nil
	}

	// 读取config.json
	data, err := os.ReadFile("config.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	// 解析JSON
	var configFile ConfigFile
	if err := json.Unmarshal(data, &configFile); err != nil {
		return nil, fmt.Errorf("failed to parse config.json: %w", err)
	}

	return &configFile, nil
}

// syncConfigToDatabase 将配置同步到数据库
func syncConfigToDatabase(database *config.Database, configFile *ConfigFile) error {
	if configFile == nil {
		return nil
	}

	log.Printf("🔄 Starting to sync config.json to database...")

	// 同步各配置项到数据库
	configs := map[string]string{
		"beta_mode":            fmt.Sprintf("%t", configFile.BetaMode),
		"api_server_port":      strconv.Itoa(configFile.APIServerPort),
		"use_default_coins":    fmt.Sprintf("%t", configFile.UseDefaultCoins),
		"coin_pool_api_url":    configFile.CoinPoolAPIURL,
		"oi_top_api_url":       configFile.OITopAPIURL,
		"max_daily_loss":       fmt.Sprintf("%.1f", configFile.MaxDailyLoss),
		"max_drawdown":         fmt.Sprintf("%.1f", configFile.MaxDrawdown),
		"stop_trading_minutes": strconv.Itoa(configFile.StopTradingMinutes),
	}

	// 同步default_coins（转换为JSON字符串存储）
	if len(configFile.DefaultCoins) > 0 {
		defaultCoinsJSON, err := json.Marshal(configFile.DefaultCoins)
		if err == nil {
			configs["default_coins"] = string(defaultCoinsJSON)
		}
	}

	// 同步杠杆配置
	if configFile.Leverage.BTCETHLeverage > 0 {
		configs["btc_eth_leverage"] = strconv.Itoa(configFile.Leverage.BTCETHLeverage)
	}
	if configFile.Leverage.AltcoinLeverage > 0 {
		configs["altcoin_leverage"] = strconv.Itoa(configFile.Leverage.AltcoinLeverage)
	}

	// 如果JWT密钥不为空，也同步
	if configFile.JWTSecret != "" {
		configs["jwt_secret"] = configFile.JWTSecret
	}

	// 更新数据库配置
	for key, value := range configs {
		if err := database.SetSystemConfig(key, value); err != nil {
			log.Printf("⚠️  Failed to update config %s: %v", key, err)
		} else {
			log.Printf("✓ Synced config: %s = %s", key, value)
		}
	}

	log.Printf("✅ config.json sync completed")
	return nil
}

// loadBetaCodesToDatabase 加载内测码文件到数据库
func loadBetaCodesToDatabase(database *config.Database) error {
	betaCodeFile := "beta_codes.txt"

	// 检查内测码文件是否存在
	if _, err := os.Stat(betaCodeFile); os.IsNotExist(err) {
		log.Printf("📄 Beta code file %s does not exist, skipping load", betaCodeFile)
		return nil
	}

	// 获取文件信息
	fileInfo, err := os.Stat(betaCodeFile)
	if err != nil {
		return fmt.Errorf("failed to get beta code file info: %w", err)
	}

	log.Printf("🔄 Found beta code file %s (%.1f KB), starting to load...", betaCodeFile, float64(fileInfo.Size())/1024)

	// 加载内测码到数据库
	err = database.LoadBetaCodesFromFile(betaCodeFile)
	if err != nil {
		return fmt.Errorf("failed to load beta codes: %w", err)
	}

	// 显示统计信息
	total, used, err := database.GetBetaCodeStats()
	if err != nil {
		log.Printf("⚠️  Failed to get beta code statistics: %v", err)
	} else {
		log.Printf("✅ Beta code loading completed: Total %d, Used %d, Remaining %d", total, used, total-used)
	}

	return nil
}

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════════╗")
	fmt.Println("║    🤖 AI Multi-Model Trading System - Supporting DeepSeek & Qwen            ║")
	fmt.Println("╚════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Load environment variables from .env file if present (for local/dev runs)
	// In Docker Compose, variables are injected by the runtime and this is harmless.
	_ = godotenv.Load()

	// 初始化数据库配置
	dbPath := "config.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	// 读取配置文件
	configFile, err := loadConfigFile()
	if err != nil {
		log.Fatalf("❌ Failed to read config.json: %v", err)
	}

	log.Printf("📋 Initializing configuration database: %s", dbPath)
	database, err := config.NewDatabase(dbPath)
	if err != nil {
		log.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer database.Close()

	// 初始化加密服务
	log.Printf("🔐 Initializing encryption service...")
	cryptoService, err := crypto.NewCryptoService("secrets/rsa_key")
	if err != nil {
		log.Fatalf("❌ Failed to initialize encryption service: %v", err)
	}
	database.SetCryptoService(cryptoService)
	log.Printf("✅ Encryption service initialized successfully")

	// 同步config.json到数据库
	if err := syncConfigToDatabase(database, configFile); err != nil {
		log.Printf("⚠️  Failed to sync config.json to database: %v", err)
	}

	// 加载内测码到数据库
	if err := loadBetaCodesToDatabase(database); err != nil {
		log.Printf("⚠️  Failed to load beta codes to database: %v", err)
	}

	// 获取系统配置
	useDefaultCoinsStr, _ := database.GetSystemConfig("use_default_coins")
	useDefaultCoins := useDefaultCoinsStr == "true"
	apiPortStr, _ := database.GetSystemConfig("api_server_port")

	// 设置JWT密钥（优先使用环境变量）
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if jwtSecret == "" {
		// 回退到数据库配置
		jwtSecret, _ = database.GetSystemConfig("jwt_secret")
		if jwtSecret == "" {
			jwtSecret = "your-jwt-secret-key-change-in-production-make-it-long-and-random"
			log.Printf("⚠️  Using default JWT secret, recommend using encryption setup script to generate secure key")
		} else {
			log.Printf("🔑 Using JWT secret from database")
		}
	} else {
		log.Printf("🔑 Using JWT secret from environment variable")
	}
	auth.SetJWTSecret(jwtSecret)

	// 管理员模式下需要管理员密码，缺失则退出

	log.Printf("✓ Configuration database initialized successfully")
	fmt.Println()

	// 从数据库读取默认主流币种列表
	defaultCoinsJSON, _ := database.GetSystemConfig("default_coins")
	var defaultCoins []string

	if defaultCoinsJSON != "" {
		// 尝试从JSON解析
		if err := json.Unmarshal([]byte(defaultCoinsJSON), &defaultCoins); err != nil {
			log.Printf("⚠️  Failed to parse default_coins config: %v, using hardcoded default values", err)
			defaultCoins = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "HYPEUSDT"}
		} else {
			log.Printf("✓ Loaded default coin list from database (%d total): %v", len(defaultCoins), defaultCoins)
		}
	} else {
		// 如果数据库中没有配置，使用硬编码默认值
		defaultCoins = []string{"BTCUSDT", "ETHUSDT", "SOLUSDT", "BNBUSDT", "XRPUSDT", "DOGEUSDT", "ADAUSDT", "HYPEUSDT"}
		log.Printf("⚠️  default_coins not configured in database, using hardcoded default values")
	}

	pool.SetDefaultCoins(defaultCoins)
	// 设置是否使用默认主流币种
	pool.SetUseDefaultCoins(useDefaultCoins)
	if useDefaultCoins {
		log.Printf("✓ Default mainstream coin list enabled")
	}

	// 设置币种池API URL
	coinPoolAPIURL, _ := database.GetSystemConfig("coin_pool_api_url")
	if coinPoolAPIURL != "" {
		pool.SetCoinPoolAPI(coinPoolAPIURL)
		log.Printf("✓ AI500 coin pool API configured")
	}

	oiTopAPIURL, _ := database.GetSystemConfig("oi_top_api_url")
	if oiTopAPIURL != "" {
		pool.SetOITopAPI(oiTopAPIURL)
		log.Printf("✓ OI Top API configured")
	}

	// 创建TraderManager
	traderManager := manager.NewTraderManager()

	// 从数据库加载所有交易员到内存
	err = traderManager.LoadTradersFromDatabase(database)
	if err != nil {
		log.Fatalf("❌ Failed to load traders: %v", err)
	}

	// 获取数据库中的所有交易员配置（用于显示，使用default用户）
	traders, err := database.GetTraders("default")
	if err != nil {
		log.Fatalf("❌ Failed to get trader list: %v", err)
	}

	// 显示加载的交易员信息
	fmt.Println()
	fmt.Println("🤖 AI Trader configurations in database:")
	if len(traders) == 0 {
		fmt.Println("  • No configured traders, please create via Web interface")
	} else {
		for _, trader := range traders {
			status := "Stopped"
			if trader.IsRunning {
				status = "Running"
			}
			fmt.Printf("  • %s (%s + %s) - Initial balance: %.0f USDT [%s]\n",
				trader.Name, strings.ToUpper(trader.AIModelID), strings.ToUpper(trader.ExchangeID),
				trader.InitialBalance, status)
		}
	}

	// 创建初始化上下文
	// TODO : 传入实际配置, 现在并未实际使用，未来所有模块初始化都将通过上下文传递配置
	// ctx := bootstrap.NewContext(&config.Config{})

	// // 执行所有初始化钩子
	// if err := bootstrap.Run(ctx); err != nil {
	// 	log.Fatalf("初始化失败: %v", err)
	// }

	fmt.Println()
	fmt.Println("🤖 AI Full Decision Mode:")
	fmt.Printf("  • AI will autonomously decide leverage for each trade (altcoins max 5x, BTC/ETH max 5x)\n")
	fmt.Println("  • AI will autonomously decide position size for each trade")
	fmt.Println("  • AI will autonomously set stop-loss and take-profit prices")
	fmt.Println("  • AI will make comprehensive analysis based on market data, technical indicators, and account status")
	fmt.Println()
	fmt.Println("⚠️  Risk Warning: AI automated trading has risks, recommend testing with small amounts!")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// 获取API服务器端口（优先级：环境变量 > 数据库配置 > 默认值）
	apiPort := 8080 // 默认端口

	// 1. 优先从环境变量 NOFX_BACKEND_PORT 读取
	if envPort := strings.TrimSpace(os.Getenv("NOFX_BACKEND_PORT")); envPort != "" {
		if port, err := strconv.Atoi(envPort); err == nil && port > 0 {
			apiPort = port
			log.Printf("🔌 Using port from environment variable: %d (NOFX_BACKEND_PORT)", apiPort)
		} else {
			log.Printf("⚠️  Environment variable NOFX_BACKEND_PORT is invalid: %s", envPort)
		}
	} else if apiPortStr != "" {
		// 2. 从数据库配置读取（config.json 同步过来的）
		if port, err := strconv.Atoi(apiPortStr); err == nil && port > 0 {
			apiPort = port
			log.Printf("🔌 Using port from database config: %d (api_server_port)", apiPort)
		}
	} else {
		log.Printf("🔌 Using default port: %d", apiPort)
	}

	// 创建并启动API服务器
	apiServer := api.NewServer(traderManager, database, cryptoService, apiPort)
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Printf("❌ API server error: %v", err)
		}
	}()

	// 启动流行情数据 - 默认使用所有交易员设置的币种 如果没有设置币种 则优先使用系统默认
	go market.NewWSMonitor(150).Start(database.GetCustomCoins())
	//go market.NewWSMonitor(150).Start([]string{}) //这里是一个使用方式 传入空的话 则使用market市场的所有币种
	// 设置优雅退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// TODO: 启动数据库中配置为运行状态的交易员
	// traderManager.StartAll()

	// 等待退出信号
	<-sigChan
	fmt.Println()
	fmt.Println()
	log.Println("📛 Received exit signal, gracefully shutting down...")

	// 步骤 1: 停止所有交易员
	log.Println("⏸️  Stopping all traders...")
	traderManager.StopAll()
	log.Println("✅ All traders stopped")

	// 步骤 2: 关闭 API 服务器
	log.Println("🛑 Stopping API server...")
	if err := apiServer.Shutdown(); err != nil {
		log.Printf("⚠️  Error shutting down API server: %v", err)
	} else {
		log.Println("✅ API server shut down safely")
	}

	// 步骤 3: 关闭数据库连接 (确保所有写入完成)
	log.Println("💾 Closing database connection...")
	if err := database.Close(); err != nil {
		log.Printf("❌ Failed to close database: %v", err)
	} else {
		log.Println("✅ Database closed safely, all data persisted")
	}

	fmt.Println()
	fmt.Println("👋 Thank you for using AI Trading System!")
}
