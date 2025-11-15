package pool

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultMainstreamCoins 默认主流币种池（从配置文件读取）
var defaultMainstreamCoins = []string{
	"BTCUSDT",
	"ETHUSDT",
	"SOLUSDT",
	"BNBUSDT",
	"XRPUSDT",
	"DOGEUSDT",
	"ADAUSDT",
	"HYPEUSDT",
}

// CoinPoolConfig 币种池配置
type CoinPoolConfig struct {
	APIURL          string
	Timeout         time.Duration
	CacheDir        string
	UseDefaultCoins bool // 是否使用默认主流币种
}

var coinPoolConfig = CoinPoolConfig{
	APIURL:          "",
	Timeout:         30 * time.Second, // 增加到30秒
	CacheDir:        "coin_pool_cache",
	UseDefaultCoins: false, // 默认不使用
}

// CoinPoolCache 币种池缓存
type CoinPoolCache struct {
	Coins      []CoinInfo `json:"coins"`
	FetchedAt  time.Time  `json:"fetched_at"`
	SourceType string     `json:"source_type"` // "api" or "cache"
}

// CoinInfo 币种信息
type CoinInfo struct {
	Pair            string  `json:"pair"`             // 交易对符号（例如：BTCUSDT）
	Score           float64 `json:"score"`            // 当前评分
	StartTime       int64   `json:"start_time"`       // 开始时间（Unix时间戳）
	StartPrice      float64 `json:"start_price"`      // 开始价格
	LastScore       float64 `json:"last_score"`       // 最新评分
	MaxScore        float64 `json:"max_score"`        // 最高评分
	MaxPrice        float64 `json:"max_price"`        // 最高价格
	IncreasePercent float64 `json:"increase_percent"` // 涨幅百分比
	IsAvailable     bool    `json:"-"`                // 是否可交易（内部使用）
}

// CoinPoolAPIResponse API返回的原始数据结构
type CoinPoolAPIResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Coins []CoinInfo `json:"coins"`
		Count int        `json:"count"`
	} `json:"data"`
}

// SetCoinPoolAPI 设置币种池API
func SetCoinPoolAPI(apiURL string) {
	coinPoolConfig.APIURL = apiURL
}

// SetOITopAPI 设置OI Top API
func SetOITopAPI(apiURL string) {
	oiTopConfig.APIURL = apiURL
}

// SetUseDefaultCoins 设置是否使用默认主流币种
func SetUseDefaultCoins(useDefault bool) {
	coinPoolConfig.UseDefaultCoins = useDefault
}

// SetDefaultCoins 设置默认主流币种列表
func SetDefaultCoins(coins []string) {
	if len(coins) > 0 {
		defaultMainstreamCoins = coins
		log.Printf("✓ Set default coin pool (%d coins): %v", len(coins), coins)
	}
}

// GetCoinPool 获取币种池列表（带重试和缓存机制）
func GetCoinPool() ([]CoinInfo, error) {
	// 优先检查是否启用默认币种列表
	if coinPoolConfig.UseDefaultCoins {
		log.Printf("✓ Enabled default mainstream coin list")
		return convertSymbolsToCoins(defaultMainstreamCoins), nil
	}

	// 检查API URL是否配置
	if strings.TrimSpace(coinPoolConfig.APIURL) == "" {
		log.Printf("⚠️  Coin pool API URL not configured, using default mainstream coin list")
		return convertSymbolsToCoins(defaultMainstreamCoins), nil
	}

	maxRetries := 3
	var lastErr error

	// 尝试从API获取
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			log.Printf("⚠️  Retry %d of %d to fetch coin pool...", attempt, maxRetries)
			time.Sleep(2 * time.Second) // 重试前等待2秒
		}

		coins, err := fetchCoinPool()
		if err == nil {
			if attempt > 1 {
				log.Printf("✓ Retry %d succeeded", attempt)
			}
			// 成功获取后保存到缓存
			if err := saveCoinPoolCache(coins); err != nil {
				log.Printf("⚠️  Failed to save coin pool cache: %v", err)
			}
			return coins, nil
		}

		lastErr = err
		log.Printf("❌ Request %d failed: %v", attempt, err)
	}

	// API获取失败，尝试使用缓存
	log.Printf("⚠️  All API requests failed, trying to use historical cache data...")
	cachedCoins, err := loadCoinPoolCache()
	if err == nil {
		log.Printf("✓ Using historical cache data (%d coins)", len(cachedCoins))
		return cachedCoins, nil
	}

	// 缓存也失败，使用默认主流币种
	log.Printf("⚠️  Cannot load cache data (last error: %v), using default mainstream coin list", lastErr)
	return convertSymbolsToCoins(defaultMainstreamCoins), nil
}

// fetchCoinPool 实际执行币种池请求
func fetchCoinPool() ([]CoinInfo, error) {
	log.Printf("🔄 Requesting AI500 coin pool...")

	client := &http.Client{
		Timeout: coinPoolConfig.Timeout,
	}

	resp, err := client.Get(coinPoolConfig.APIURL)
	if err != nil {
		return nil, fmt.Errorf("failed to request coin pool API: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned error (status %d): %s", resp.StatusCode, string(body))
	}

	// 解析API响应
	var response CoinPoolAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("JSON parsing failed: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("API returned failure status")
	}

	if len(response.Data.Coins) == 0 {
		return nil, fmt.Errorf("coin list is empty")
	}

	// 设置IsAvailable标志
	coins := response.Data.Coins
	for i := range coins {
		coins[i].IsAvailable = true
	}

	log.Printf("✓ Successfully fetched %d coins", len(coins))
	return coins, nil
}

// saveCoinPoolCache 保存币种池到缓存文件
func saveCoinPoolCache(coins []CoinInfo) error {
	// 确保缓存目录存在
	if err := os.MkdirAll(coinPoolConfig.CacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	cache := CoinPoolCache{
		Coins:      coins,
		FetchedAt:  time.Now(),
		SourceType: "api",
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize cache data: %w", err)
	}

	cachePath := filepath.Join(coinPoolConfig.CacheDir, "latest.json")
	if err := ioutil.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	log.Printf("💾 Saved coin pool cache (%d coins)", len(coins))
	return nil
}

// loadCoinPoolCache 从缓存文件加载币种池
func loadCoinPoolCache() ([]CoinInfo, error) {
	cachePath := filepath.Join(coinPoolConfig.CacheDir, "latest.json")

	// 检查文件是否存在
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("cache file does not exist")
	}

	data, err := ioutil.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cache CoinPoolCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache data: %w", err)
	}

	// 检查缓存年龄
	cacheAge := time.Since(cache.FetchedAt)
	if cacheAge > 24*time.Hour {
		log.Printf("⚠️  Cache data is old (%.1f hours ago), but still usable", cacheAge.Hours())
	} else {
		log.Printf("📂 Cache data time: %s (%.1f minutes ago)",
			cache.FetchedAt.Format("2006-01-02 15:04:05"),
			cacheAge.Minutes())
	}

	return cache.Coins, nil
}

// GetAvailableCoins 获取可用的币种列表（过滤不可用的）
func GetAvailableCoins() ([]string, error) {
	coins, err := GetCoinPool()
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, coin := range coins {
		if coin.IsAvailable {
			// 确保symbol格式正确（转为大写USDT交易对）
			symbol := normalizeSymbol(coin.Pair)
			symbols = append(symbols, symbol)
		}
	}

	if len(symbols) == 0 {
		return nil, fmt.Errorf("no available coins")
	}

	return symbols, nil
}

// GetTopRatedCoins 获取评分最高的N个币种（按评分从大到小排序）
func GetTopRatedCoins(limit int) ([]string, error) {
	coins, err := GetCoinPool()
	if err != nil {
		return nil, err
	}

	// 过滤可用的币种
	var availableCoins []CoinInfo
	for _, coin := range coins {
		if coin.IsAvailable {
			availableCoins = append(availableCoins, coin)
		}
	}

	if len(availableCoins) == 0 {
		return nil, fmt.Errorf("no available coins")
	}

	// 按Score降序排序（冒泡排序）
	for i := 0; i < len(availableCoins); i++ {
		for j := i + 1; j < len(availableCoins); j++ {
			if availableCoins[i].Score < availableCoins[j].Score {
				availableCoins[i], availableCoins[j] = availableCoins[j], availableCoins[i]
			}
		}
	}

	// 取前N个
	maxCount := limit
	if len(availableCoins) < maxCount {
		maxCount = len(availableCoins)
	}

	var symbols []string
	for i := 0; i < maxCount; i++ {
		symbol := normalizeSymbol(availableCoins[i].Pair)
		symbols = append(symbols, symbol)
	}

	return symbols, nil
}

// normalizeSymbol 标准化币种符号
func normalizeSymbol(symbol string) string {
	// 移除空格
	symbol = trimSpaces(symbol)

	// 转为大写
	symbol = toUpper(symbol)

	// 确保以USDT结尾
	if !endsWith(symbol, "USDT") {
		symbol = symbol + "USDT"
	}

	return symbol
}

// 辅助函数
func trimSpaces(s string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			result += string(s[i])
		}
	}
	return result
}

func toUpper(s string) string {
	result := ""
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c = c - 'a' + 'A'
		}
		result += string(c)
	}
	return result
}

func endsWith(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

// convertSymbolsToCoins 将币种符号列表转换为CoinInfo列表
func convertSymbolsToCoins(symbols []string) []CoinInfo {
	coins := make([]CoinInfo, 0, len(symbols))
	for _, symbol := range symbols {
		coins = append(coins, CoinInfo{
			Pair:        symbol,
			Score:       0,
			IsAvailable: true,
		})
	}
	return coins
}

// ========== OI Top（持仓量增长Top20）数据 ==========

// OIPosition 持仓量数据
type OIPosition struct {
	Symbol            string  `json:"symbol"`
	Rank              int     `json:"rank"`
	CurrentOI         float64 `json:"current_oi"`          // 当前持仓量
	OIDelta           float64 `json:"oi_delta"`            // 持仓量变化
	OIDeltaPercent    float64 `json:"oi_delta_percent"`    // 持仓量变化百分比
	OIDeltaValue      float64 `json:"oi_delta_value"`      // 持仓量变化价值
	PriceDeltaPercent float64 `json:"price_delta_percent"` // 价格变化百分比
	NetLong           float64 `json:"net_long"`            // 净多仓
	NetShort          float64 `json:"net_short"`           // 净空仓
}

// OITopAPIResponse OI Top API返回的数据结构
type OITopAPIResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Positions []OIPosition `json:"positions"`
		Count     int          `json:"count"`
		Exchange  string       `json:"exchange"`
		TimeRange string       `json:"time_range"`
	} `json:"data"`
}

// OITopCache OI Top 缓存
type OITopCache struct {
	Positions  []OIPosition `json:"positions"`
	FetchedAt  time.Time    `json:"fetched_at"`
	SourceType string       `json:"source_type"`
}

var oiTopConfig = struct {
	APIURL   string
	Timeout  time.Duration
	CacheDir string
}{
	APIURL:   "",
	Timeout:  30 * time.Second,
	CacheDir: "coin_pool_cache",
}

// GetOITopPositions 获取持仓量增长Top20数据（带重试和缓存）
func GetOITopPositions() ([]OIPosition, error) {
	// 检查API URL是否配置
	if strings.TrimSpace(oiTopConfig.APIURL) == "" {
		log.Printf("⚠️  OI Top API URL not configured, skipping OI Top data fetch")
		return []OIPosition{}, nil // 返回空列表，不是错误
	}

	maxRetries := 3
	var lastErr error

	// 尝试从API获取
	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			log.Printf("⚠️  Retry %d of %d to fetch OI Top data...", attempt, maxRetries)
			time.Sleep(2 * time.Second)
		}

		positions, err := fetchOITop()
		if err == nil {
			if attempt > 1 {
				log.Printf("✓ Retry %d succeeded", attempt)
			}
			// 成功获取后保存到缓存
			if err := saveOITopCache(positions); err != nil {
				log.Printf("⚠️  Failed to save OI Top cache: %v", err)
			}
			return positions, nil
		}

		lastErr = err
		log.Printf("❌ OI Top request %d failed: %v", attempt, err)
	}

	// API获取失败，尝试使用缓存
	log.Printf("⚠️  All OI Top API requests failed, trying to use historical cache data...")
	cachedPositions, err := loadOITopCache()
	if err == nil {
		log.Printf("✓ Using historical OI Top cache data (%d coins)", len(cachedPositions))
		return cachedPositions, nil
	}

	// 缓存也失败，返回空列表（OI Top是可选的）
	log.Printf("⚠️  Cannot load OI Top cache data (last error: %v), skipping OI Top data", lastErr)
	return []OIPosition{}, nil
}

// fetchOITop 实际执行OI Top请求
func fetchOITop() ([]OIPosition, error) {
	log.Printf("🔄 Requesting OI Top data...")

	client := &http.Client{
		Timeout: oiTopConfig.Timeout,
	}

	resp, err := client.Get(oiTopConfig.APIURL)
	if err != nil {
		return nil, fmt.Errorf("failed to request OI Top API: %w", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read OI Top response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OI Top API returned error (status %d): %s", resp.StatusCode, string(body))
	}

	// 解析API响应
	var response OITopAPIResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("OI Top JSON parsing failed: %w", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("OI Top API returned failure status")
	}

	if len(response.Data.Positions) == 0 {
		return nil, fmt.Errorf("OI Top position list is empty")
	}

	log.Printf("✓ Successfully fetched %d OI Top coins (time range: %s)",
		len(response.Data.Positions), response.Data.TimeRange)
	return response.Data.Positions, nil
}

// saveOITopCache 保存OI Top数据到缓存
func saveOITopCache(positions []OIPosition) error {
	if err := os.MkdirAll(oiTopConfig.CacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	cache := OITopCache{
		Positions:  positions,
		FetchedAt:  time.Now(),
		SourceType: "api",
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize OI Top cache data: %w", err)
	}

	cachePath := filepath.Join(oiTopConfig.CacheDir, "oi_top_latest.json")
	if err := ioutil.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write OI Top cache file: %w", err)
	}

	log.Printf("💾 Saved OI Top cache (%d coins)", len(positions))
	return nil
}

// loadOITopCache 从缓存加载OI Top数据
func loadOITopCache() ([]OIPosition, error) {
	cachePath := filepath.Join(oiTopConfig.CacheDir, "oi_top_latest.json")

	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("OI Top cache file does not exist")
	}

	data, err := ioutil.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read OI Top cache file: %w", err)
	}

	var cache OITopCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse OI Top cache data: %w", err)
	}

	cacheAge := time.Since(cache.FetchedAt)
	if cacheAge > 24*time.Hour {
		log.Printf("⚠️  OI Top cache data is old (%.1f hours ago), but still usable", cacheAge.Hours())
	} else {
		log.Printf("📂 OI Top cache data time: %s (%.1f minutes ago)",
			cache.FetchedAt.Format("2006-01-02 15:04:05"),
			cacheAge.Minutes())
	}

	return cache.Positions, nil
}

// GetOITopSymbols 获取OI Top的币种符号列表
func GetOITopSymbols() ([]string, error) {
	positions, err := GetOITopPositions()
	if err != nil {
		return nil, err
	}

	var symbols []string
	for _, pos := range positions {
		symbol := normalizeSymbol(pos.Symbol)
		symbols = append(symbols, symbol)
	}

	return symbols, nil
}

// MergedCoinPool 合并的币种池（AI500 + OI Top）
type MergedCoinPool struct {
	AI500Coins    []CoinInfo          // AI500评分币种
	OITopCoins    []OIPosition        // 持仓量增长Top20
	AllSymbols    []string            // 所有不重复的币种符号
	SymbolSources map[string][]string // 每个币种的来源（"ai500"/"oi_top"）
}

// GetMergedCoinPool 获取合并后的币种池（AI500 + OI Top，去重）
func GetMergedCoinPool(ai500Limit int) (*MergedCoinPool, error) {
	// 1. 获取AI500数据
	ai500TopSymbols, err := GetTopRatedCoins(ai500Limit)
	if err != nil {
		log.Printf("⚠️  Failed to fetch AI500 data: %v", err)
		ai500TopSymbols = []string{} // 失败时用空列表
	}

	// 2. 获取OI Top数据
	oiTopSymbols, err := GetOITopSymbols()
	if err != nil {
		log.Printf("⚠️  Failed to fetch OI Top data: %v", err)
		oiTopSymbols = []string{} // 失败时用空列表
	}

	// 3. 合并并去重
	symbolSet := make(map[string]bool)
	symbolSources := make(map[string][]string)

	// 添加AI500币种
	for _, symbol := range ai500TopSymbols {
		symbolSet[symbol] = true
		symbolSources[symbol] = append(symbolSources[symbol], "ai500")
	}

	// 添加OI Top币种
	for _, symbol := range oiTopSymbols {
		if !symbolSet[symbol] {
			symbolSet[symbol] = true
		}
		symbolSources[symbol] = append(symbolSources[symbol], "oi_top")
	}

	// 转换为数组
	var allSymbols []string
	for symbol := range symbolSet {
		allSymbols = append(allSymbols, symbol)
	}

	// 获取完整数据
	ai500Coins, _ := GetCoinPool()
	oiTopPositions, _ := GetOITopPositions()

	merged := &MergedCoinPool{
		AI500Coins:    ai500Coins,
		OITopCoins:    oiTopPositions,
		AllSymbols:    allSymbols,
		SymbolSources: symbolSources,
	}

	log.Printf("📊 Coin pool merge completed: AI500=%d, OI_Top=%d, Total (deduplicated)=%d",
		len(ai500TopSymbols), len(oiTopSymbols), len(allSymbols))

	return merged, nil
}
