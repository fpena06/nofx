package trader

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sonirico/go-hyperliquid"
)

// HyperliquidTrader Hyperliquid交易器
type HyperliquidTrader struct {
	exchange      *hyperliquid.Exchange
	ctx           context.Context
	walletAddr    string
	meta          *hyperliquid.Meta // 缓存meta信息（包含精度等）
	metaMutex     sync.RWMutex      // 保护meta字段的并发访问
	isCrossMargin bool              // 是否为全仓模式
}

// NewHyperliquidTrader 创建Hyperliquid交易器
func NewHyperliquidTrader(privateKeyHex string, walletAddr string, testnet bool) (*HyperliquidTrader, error) {
	// 去掉私钥的 0x 前缀（如果有，不区分大小写）
	privateKeyHex = strings.TrimPrefix(strings.ToLower(privateKeyHex), "0x")

	// 解析私钥
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// 选择API URL
	apiURL := hyperliquid.MainnetAPIURL
	if testnet {
		apiURL = hyperliquid.TestnetAPIURL
	}

	// Security enhancement: Implement Agent Wallet best practices
	// Reference: https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/nonces-and-api-wallets
	agentAddr := crypto.PubkeyToAddress(*privateKey.Public().(*ecdsa.PublicKey)).Hex()

	if walletAddr == "" {
		return nil, fmt.Errorf("❌ Configuration error: Main wallet address (hyperliquid_wallet_addr) not provided\n" +
			"🔐 Correct configuration pattern:\n" +
			"  1. hyperliquid_private_key = Agent Private Key (for signing only, balance should be ~0)\n" +
			"  2. hyperliquid_wallet_addr = Main Wallet Address (holds funds, never expose private key)\n" +
			"💡 Please create an Agent Wallet on Hyperliquid official website and authorize it before configuration:\n" +
			"   https://app.hyperliquid.xyz/ → Settings → API Wallets")
	}

	// Check if user accidentally uses main wallet private key (security risk)
	if strings.EqualFold(walletAddr, agentAddr) {
		log.Printf("⚠️⚠️⚠️ WARNING: Main wallet address (%s) matches Agent wallet address!", walletAddr)
		log.Printf("   This indicates you may be using your main wallet private key, which poses extremely high security risks!")
		log.Printf("   Recommendation: Immediately create a separate Agent Wallet on Hyperliquid official website")
		log.Printf("   Reference: https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/nonces-and-api-wallets")
	} else {
		log.Printf("✓ Using Agent Wallet mode (secure)")
		log.Printf("  └─ Agent wallet address: %s (for signing)", agentAddr)
		log.Printf("  └─ Main wallet address: %s (holds funds)", walletAddr)
	}

	ctx := context.Background()

	// 创建Exchange客户端（Exchange包含Info功能）
	exchange := hyperliquid.NewExchange(
		ctx,
		privateKey,
		apiURL,
		nil,        // Meta will be fetched automatically
		"",         // vault address (empty for personal account)
		walletAddr, // wallet address
		nil,        // SpotMeta will be fetched automatically
	)

	log.Printf("✓ Hyperliquid trader initialized successfully (testnet=%v, wallet=%s)", testnet, walletAddr)

	// 获取meta信息（包含精度等配置）
	meta, err := exchange.Info().Meta(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve meta information: %w", err)
	}

	// 🔍 Security check: Validate Agent wallet balance (should be close to 0)
	// Only check if using separate Agent wallet (not when main wallet is used as agent)
	if !strings.EqualFold(walletAddr, agentAddr) {
		agentState, err := exchange.Info().UserState(ctx, agentAddr)
		if err == nil && agentState != nil && agentState.CrossMarginSummary.AccountValue != "" {
			// Parse Agent wallet balance
			agentBalance, _ := strconv.ParseFloat(agentState.CrossMarginSummary.AccountValue, 64)

			if agentBalance > 100 {
				// Critical: Agent wallet holds too much funds
				log.Printf("🚨🚨🚨 CRITICAL SECURITY WARNING 🚨🚨🚨")
				log.Printf("   Agent wallet balance: %.2f USDC (exceeds safe threshold of 100 USDC)", agentBalance)
				log.Printf("   Agent wallet address: %s", agentAddr)
				log.Printf("   ⚠️  Agent wallets should only be used for signing and hold minimal/zero balance")
				log.Printf("   ⚠️  High balance in Agent wallet poses security risks")
				log.Printf("   📖 Reference: https://hyperliquid.gitbook.io/hyperliquid-docs/for-developers/api/nonces-and-api-wallets")
				log.Printf("   💡 Recommendation: Transfer funds to main wallet and keep Agent wallet balance near 0")
				return nil, fmt.Errorf("security check failed: Agent wallet balance too high (%.2f USDC), exceeds 100 USDC threshold", agentBalance)
			} else if agentBalance > 10 {
				// Warning: Agent wallet has some balance (acceptable but not ideal)
				log.Printf("⚠️  Notice: Agent wallet address (%s) has some balance: %.2f USDC", agentAddr, agentBalance)
				log.Printf("   While not critical, it's recommended to keep Agent wallet balance near 0 for security")
			} else {
				// OK: Agent wallet balance is safe
				log.Printf("✓ Agent wallet balance is safe: %.2f USDC (near zero as recommended)", agentBalance)
			}
		} else if err != nil {
			// Failed to query agent balance - log warning but don't block initialization
			log.Printf("⚠️  Could not verify Agent wallet balance (query failed): %v", err)
			log.Printf("   Proceeding with initialization, but please manually verify Agent wallet balance is near 0")
		}
	}

	return &HyperliquidTrader{
		exchange:      exchange,
		ctx:           ctx,
		walletAddr:    walletAddr,
		meta:          meta,
		isCrossMargin: true, // 默认使用全仓模式
	}, nil
}

// GetBalance 获取账户余额
func (t *HyperliquidTrader) GetBalance() (map[string]interface{}, error) {
	log.Printf("🔄 Calling Hyperliquid API to retrieve account balance...")

	// ✅ Step 1: 查询 Spot 现货账户余额
	spotState, err := t.exchange.Info().SpotUserState(t.ctx, t.walletAddr)
	var spotUSDCBalance float64 = 0.0
	if err != nil {
		log.Printf("⚠️ Failed to query Spot balance (possibly no spot assets): %v", err)
	} else if spotState != nil && len(spotState.Balances) > 0 {
		for _, balance := range spotState.Balances {
			if balance.Coin == "USDC" {
				spotUSDCBalance, _ = strconv.ParseFloat(balance.Total, 64)
				log.Printf("✓ Found Spot balance: %.2f USDC", spotUSDCBalance)
				break
			}
		}
	}

	// ✅ Step 2: 查询 Perpetuals 合约账户状态
	accountState, err := t.exchange.Info().UserState(t.ctx, t.walletAddr)
	if err != nil {
		log.Printf("❌ Hyperliquid Perpetuals API call failed: %v", err)
		return nil, fmt.Errorf("failed to retrieve account information: %w", err)
	}

	// 解析余额信息（MarginSummary字段都是string）
	result := make(map[string]interface{})

	// ✅ Step 3: 根据保证金模式动态选择正确的摘要（CrossMarginSummary 或 MarginSummary）
	var accountValue, totalMarginUsed float64
	var summaryType string
	var summary interface{}

	if t.isCrossMargin {
		// 全仓模式：使用 CrossMarginSummary
		accountValue, _ = strconv.ParseFloat(accountState.CrossMarginSummary.AccountValue, 64)
		totalMarginUsed, _ = strconv.ParseFloat(accountState.CrossMarginSummary.TotalMarginUsed, 64)
		summaryType = "CrossMarginSummary (cross margin)"
		summary = accountState.CrossMarginSummary
	} else {
		// 逐仓模式：使用 MarginSummary
		accountValue, _ = strconv.ParseFloat(accountState.MarginSummary.AccountValue, 64)
		totalMarginUsed, _ = strconv.ParseFloat(accountState.MarginSummary.TotalMarginUsed, 64)
		summaryType = "MarginSummary (isolated margin)"
		summary = accountState.MarginSummary
	}

	// 🔍 调试：打印API返回的完整摘要结构
	summaryJSON, _ := json.MarshalIndent(summary, "  ", "  ")
	log.Printf("🔍 [DEBUG] Hyperliquid API %s complete data:", summaryType)
	log.Printf("%s", string(summaryJSON))

	// ⚠️ 关键修复：从所有持仓中累加真正的未实现盈亏
	totalUnrealizedPnl := 0.0
	for _, assetPos := range accountState.AssetPositions {
		unrealizedPnl, _ := strconv.ParseFloat(assetPos.Position.UnrealizedPnl, 64)
		totalUnrealizedPnl += unrealizedPnl
	}

	// ✅ 正确理解Hyperliquid字段：
	// AccountValue = 总账户净值（已包含空闲资金+持仓价值+未实现盈亏）
	// TotalMarginUsed = 持仓占用的保证金（已包含在AccountValue中，仅用于显示）
	//
	// 为了兼容auto_trader.go的计算逻辑（totalEquity = totalWalletBalance + totalUnrealizedProfit）
	// 需要返回"不包含未实现盈亏的钱包余额"
	walletBalanceWithoutUnrealized := accountValue - totalUnrealizedPnl

	// ✅ Step 4: 使用 Withdrawable 欄位（PR #443）
	// Withdrawable 是官方提供的真实可提现余额，比简单计算更可靠
	availableBalance := 0.0
	if accountState.Withdrawable != "" {
		withdrawable, err := strconv.ParseFloat(accountState.Withdrawable, 64)
		if err == nil && withdrawable > 0 {
			availableBalance = withdrawable
			log.Printf("✓ Using Withdrawable as available balance: %.2f", availableBalance)
		}
	}

	// 降级方案：如果没有 Withdrawable，使用简单计算
	if availableBalance == 0 && accountState.Withdrawable == "" {
		availableBalance = accountValue - totalMarginUsed
		if availableBalance < 0 {
			log.Printf("⚠️ Calculated available balance is negative (%.2f), resetting to 0", availableBalance)
			availableBalance = 0
		}
	}

	// ✅ Step 5: 正确处理 Spot + Perpetuals 余额
	// 重要：Spot 只加到总资产，不加到可用余额
	//      原因：Spot 和 Perpetuals 是独立帐户，需手动 ClassTransfer 才能转账
	totalWalletBalance := walletBalanceWithoutUnrealized + spotUSDCBalance

	result["totalWalletBalance"] = totalWalletBalance    // 总资产（Perp + Spot）
	result["availableBalance"] = availableBalance        // 可用余额（仅 Perpetuals，不含 Spot）
	result["totalUnrealizedProfit"] = totalUnrealizedPnl // 未实现盈亏（仅来自 Perpetuals）
	result["spotBalance"] = spotUSDCBalance              // Spot 现货余额（单独返回）

	log.Printf("✓ Hyperliquid complete account:")
	log.Printf("  • Spot balance: %.2f USDC (requires manual transfer to Perpetuals to open positions)", spotUSDCBalance)
	log.Printf("  • Perpetuals contract value: %.2f USDC (wallet %.2f + unrealized %.2f)",
		accountValue,
		walletBalanceWithoutUnrealized,
		totalUnrealizedPnl)
	log.Printf("  • Perpetuals available balance: %.2f USDC (can be used directly for opening positions)", availableBalance)
	log.Printf("  • Margin used: %.2f USDC", totalMarginUsed)
	log.Printf("  • Total assets (Perp+Spot): %.2f USDC", totalWalletBalance)
	log.Printf("  ⭐ Total assets: %.2f USDC | Perp available: %.2f USDC | Spot balance: %.2f USDC",
		totalWalletBalance, availableBalance, spotUSDCBalance)

	return result, nil
}

// GetPositions 获取所有持仓
func (t *HyperliquidTrader) GetPositions() ([]map[string]interface{}, error) {
	// 获取账户状态
	accountState, err := t.exchange.Info().UserState(t.ctx, t.walletAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve positions: %w", err)
	}

	var result []map[string]interface{}

	// 遍历所有持仓
	for _, assetPos := range accountState.AssetPositions {
		position := assetPos.Position

		// 持仓数量（string类型）
		posAmt, _ := strconv.ParseFloat(position.Szi, 64)

		if posAmt == 0 {
			continue // 跳过无持仓的
		}

		posMap := make(map[string]interface{})

		// 标准化symbol格式（Hyperliquid使用如"BTC"，我们转换为"BTCUSDT"）
		symbol := position.Coin + "USDT"
		posMap["symbol"] = symbol

		// 持仓数量和方向
		if posAmt > 0 {
			posMap["side"] = "long"
			posMap["positionAmt"] = posAmt
		} else {
			posMap["side"] = "short"
			posMap["positionAmt"] = -posAmt // 转为正数
		}

		// 价格信息（EntryPx和LiquidationPx是指针类型）
		var entryPrice, liquidationPx float64
		if position.EntryPx != nil {
			entryPrice, _ = strconv.ParseFloat(*position.EntryPx, 64)
		}
		if position.LiquidationPx != nil {
			liquidationPx, _ = strconv.ParseFloat(*position.LiquidationPx, 64)
		}

		positionValue, _ := strconv.ParseFloat(position.PositionValue, 64)
		unrealizedPnl, _ := strconv.ParseFloat(position.UnrealizedPnl, 64)

		// 计算mark price（positionValue / abs(posAmt)）
		var markPrice float64
		if posAmt != 0 {
			markPrice = positionValue / absFloat(posAmt)
		}

		posMap["entryPrice"] = entryPrice
		posMap["markPrice"] = markPrice
		posMap["unRealizedProfit"] = unrealizedPnl
		posMap["leverage"] = float64(position.Leverage.Value)
		posMap["liquidationPrice"] = liquidationPx

		result = append(result, posMap)
	}

	return result, nil
}

// SetMarginMode 设置仓位模式 (在SetLeverage时一并设置)
func (t *HyperliquidTrader) SetMarginMode(symbol string, isCrossMargin bool) error {
	// Hyperliquid的仓位模式在SetLeverage时设置，这里只记录
	t.isCrossMargin = isCrossMargin
	marginModeStr := "cross margin"
	if !isCrossMargin {
		marginModeStr = "isolated margin"
	}
	log.Printf("  ✓ %s will use %s mode", symbol, marginModeStr)
	return nil
}

// SetLeverage 设置杠杆
func (t *HyperliquidTrader) SetLeverage(symbol string, leverage int) error {
	// Hyperliquid symbol格式（去掉USDT后缀）
	coin := convertSymbolToHyperliquid(symbol)

	// 调用UpdateLeverage (leverage int, name string, isCross bool)
	// 第三个参数: true=全仓模式, false=逐仓模式
	_, err := t.exchange.UpdateLeverage(t.ctx, leverage, coin, t.isCrossMargin)
	if err != nil {
		return fmt.Errorf("failed to set leverage: %w", err)
	}

	log.Printf("  ✓ %s leverage switched to %dx", symbol, leverage)
	return nil
}

// refreshMetaIfNeeded 当 Meta 信息失效时刷新（Asset ID 为 0 时触发）
func (t *HyperliquidTrader) refreshMetaIfNeeded(coin string) error {
	assetID := t.exchange.Info().NameToAsset(coin)
	if assetID != 0 {
		return nil // Meta 正常，无需刷新
	}

	log.Printf("⚠️  %s's Asset ID is 0, attempting to refresh Meta information...", coin)

	// 刷新 Meta 信息
	meta, err := t.exchange.Info().Meta(t.ctx)
	if err != nil {
		return fmt.Errorf("failed to refresh Meta information: %w", err)
	}

	// ✅ 并发安全：使用写锁保护 meta 字段更新
	t.metaMutex.Lock()
	t.meta = meta
	t.metaMutex.Unlock()

	log.Printf("✅ Meta information refreshed, contains %d assets", len(meta.Universe))

	// 验证刷新后的 Asset ID
	assetID = t.exchange.Info().NameToAsset(coin)
	if assetID == 0 {
		return fmt.Errorf("❌ Asset %s's Asset ID is still 0 even after Meta refresh. Possible reasons:\n"+
			"  1. This coin is not listed on Hyperliquid\n"+
			"  2. Incorrect coin name (should be BTC instead of BTCUSDT)\n"+
			"  3. API connection issue", coin)
	}

	log.Printf("✅ Asset ID check passed after refresh: %s -> %d", coin, assetID)
	return nil
}

// OpenLong 开多仓
func (t *HyperliquidTrader) OpenLong(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ Failed to cancel old orders: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// Hyperliquid symbol格式
	coin := convertSymbolToHyperliquid(symbol)

	// 获取当前价格（用于市价单）
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return nil, err
	}

	// ⚠️ 关键：根据币种精度要求，四舍五入数量
	roundedQuantity := t.roundToSzDecimals(coin, quantity)
	log.Printf("  📏 Quantity precision handling: %.8f -> %.8f (szDecimals=%d)", quantity, roundedQuantity, t.getSzDecimals(coin))

	// ⚠️ 关键：价格也需要处理为5位有效数字
	aggressivePrice := t.roundPriceToSigfigs(price * 1.01)
	log.Printf("  💰 Price precision handling: %.8f -> %.8f (5 significant figures)", price*1.01, aggressivePrice)

	// 创建市价买入订单（使用IOC limit order with aggressive price）
	order := hyperliquid.CreateOrderRequest{
		Coin:  coin,
		IsBuy: true,
		Size:  roundedQuantity, // 使用四舍五入后的数量
		Price: aggressivePrice, // 使用处理后的价格
		OrderType: hyperliquid.OrderType{
			Limit: &hyperliquid.LimitOrderType{
				Tif: hyperliquid.TifIoc, // Immediate or Cancel (类似市价单)
			},
		},
		ReduceOnly: false,
	}

	_, err = t.exchange.Order(t.ctx, order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open long position: %w", err)
	}

	log.Printf("✓ Long position opened successfully: %s quantity: %.4f", symbol, roundedQuantity)

	result := make(map[string]interface{})
	result["orderId"] = 0 // Hyperliquid没有返回order ID
	result["symbol"] = symbol
	result["status"] = "FILLED"

	return result, nil
}

// OpenShort 开空仓
func (t *HyperliquidTrader) OpenShort(symbol string, quantity float64, leverage int) (map[string]interface{}, error) {
	// 先取消该币种的所有委托单
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ Failed to cancel old orders: %v", err)
	}

	// 设置杠杆
	if err := t.SetLeverage(symbol, leverage); err != nil {
		return nil, err
	}

	// Hyperliquid symbol格式
	coin := convertSymbolToHyperliquid(symbol)

	// 获取当前价格
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return nil, err
	}

	// ⚠️ 关键：根据币种精度要求，四舍五入数量
	roundedQuantity := t.roundToSzDecimals(coin, quantity)
	log.Printf("  📏 Quantity precision handling: %.8f -> %.8f (szDecimals=%d)", quantity, roundedQuantity, t.getSzDecimals(coin))

	// ⚠️ 关键：价格也需要处理为5位有效数字
	aggressivePrice := t.roundPriceToSigfigs(price * 0.99)
	log.Printf("  💰 Price precision handling: %.8f -> %.8f (5 significant figures)", price*0.99, aggressivePrice)

	// 创建市价卖出订单
	order := hyperliquid.CreateOrderRequest{
		Coin:  coin,
		IsBuy: false,
		Size:  roundedQuantity, // 使用四舍五入后的数量
		Price: aggressivePrice, // 使用处理后的价格
		OrderType: hyperliquid.OrderType{
			Limit: &hyperliquid.LimitOrderType{
				Tif: hyperliquid.TifIoc,
			},
		},
		ReduceOnly: false,
	}

	_, err = t.exchange.Order(t.ctx, order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to open short position: %w", err)
	}

	log.Printf("✓ Short position opened successfully: %s quantity: %.4f", symbol, roundedQuantity)

	result := make(map[string]interface{})
	result["orderId"] = 0
	result["symbol"] = symbol
	result["status"] = "FILLED"

	return result, nil
}

// CloseLong 平多仓
func (t *HyperliquidTrader) CloseLong(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "long" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("no long position found for %s", symbol)
		}
	}

	// Hyperliquid symbol格式
	coin := convertSymbolToHyperliquid(symbol)

	// 获取当前价格
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return nil, err
	}

	// ⚠️ 关键：根据币种精度要求，四舍五入数量
	roundedQuantity := t.roundToSzDecimals(coin, quantity)
	log.Printf("  📏 Quantity precision handling: %.8f -> %.8f (szDecimals=%d)", quantity, roundedQuantity, t.getSzDecimals(coin))

	// ⚠️ 关键：价格也需要处理为5位有效数字
	aggressivePrice := t.roundPriceToSigfigs(price * 0.99)
	log.Printf("  💰 Price precision handling: %.8f -> %.8f (5 significant figures)", price*0.99, aggressivePrice)

	// 创建平仓订单（卖出 + ReduceOnly）
	order := hyperliquid.CreateOrderRequest{
		Coin:  coin,
		IsBuy: false,
		Size:  roundedQuantity, // 使用四舍五入后的数量
		Price: aggressivePrice, // 使用处理后的价格
		OrderType: hyperliquid.OrderType{
			Limit: &hyperliquid.LimitOrderType{
				Tif: hyperliquid.TifIoc,
			},
		},
		ReduceOnly: true, // 只平仓，不开新仓
	}

	_, err = t.exchange.Order(t.ctx, order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to close long position: %w", err)
	}

	log.Printf("✓ Long position closed successfully: %s quantity: %.4f", symbol, roundedQuantity)

	// 平仓后取消该币种的所有挂单
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ Failed to cancel pending orders: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = 0
	result["symbol"] = symbol
	result["status"] = "FILLED"

	return result, nil
}

// CloseShort 平空仓
func (t *HyperliquidTrader) CloseShort(symbol string, quantity float64) (map[string]interface{}, error) {
	// 如果数量为0，获取当前持仓数量
	if quantity == 0 {
		positions, err := t.GetPositions()
		if err != nil {
			return nil, err
		}

		for _, pos := range positions {
			if pos["symbol"] == symbol && pos["side"] == "short" {
				quantity = pos["positionAmt"].(float64)
				break
			}
		}

		if quantity == 0 {
			return nil, fmt.Errorf("no short position found for %s", symbol)
		}
	}

	// Hyperliquid symbol格式
	coin := convertSymbolToHyperliquid(symbol)

	// 获取当前价格
	price, err := t.GetMarketPrice(symbol)
	if err != nil {
		return nil, err
	}

	// ⚠️ 关键：根据币种精度要求，四舍五入数量
	roundedQuantity := t.roundToSzDecimals(coin, quantity)
	log.Printf("  📏 Quantity precision handling: %.8f -> %.8f (szDecimals=%d)", quantity, roundedQuantity, t.getSzDecimals(coin))

	// ⚠️ 关键：价格也需要处理为5位有效数字
	aggressivePrice := t.roundPriceToSigfigs(price * 1.01)
	log.Printf("  💰 Price precision handling: %.8f -> %.8f (5 significant figures)", price*1.01, aggressivePrice)

	// 创建平仓订单（买入 + ReduceOnly）
	order := hyperliquid.CreateOrderRequest{
		Coin:  coin,
		IsBuy: true,
		Size:  roundedQuantity, // 使用四舍五入后的数量
		Price: aggressivePrice, // 使用处理后的价格
		OrderType: hyperliquid.OrderType{
			Limit: &hyperliquid.LimitOrderType{
				Tif: hyperliquid.TifIoc,
			},
		},
		ReduceOnly: true,
	}

	_, err = t.exchange.Order(t.ctx, order, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to close short position: %w", err)
	}

	log.Printf("✓ Short position closed successfully: %s quantity: %.4f", symbol, roundedQuantity)

	// 平仓后取消该币种的所有挂单
	if err := t.CancelAllOrders(symbol); err != nil {
		log.Printf("  ⚠ Failed to cancel pending orders: %v", err)
	}

	result := make(map[string]interface{})
	result["orderId"] = 0
	result["symbol"] = symbol
	result["status"] = "FILLED"

	return result, nil
}

// CancelStopOrders 取消该币种的止盈/止

// CancelStopLossOrders 仅取消止损单（Hyperliquid 暂无法区分止损和止盈，取消所有）
func (t *HyperliquidTrader) CancelStopLossOrders(symbol string) error {
	// Hyperliquid SDK 的 OpenOrder 结构不暴露 trigger 字段
	// 无法区分止损和止盈单，因此取消该币种的所有挂单
	log.Printf("  ⚠️ Hyperliquid cannot distinguish between stop-loss/take-profit orders, will cancel all pending orders")
	return t.CancelStopOrders(symbol)
}

// CancelTakeProfitOrders 仅取消止盈单（Hyperliquid 暂无法区分止损和止盈，取消所有）
func (t *HyperliquidTrader) CancelTakeProfitOrders(symbol string) error {
	// Hyperliquid SDK 的 OpenOrder 结构不暴露 trigger 字段
	// 无法区分止损和止盈单，因此取消该币种的所有挂单
	log.Printf("  ⚠️ Hyperliquid cannot distinguish between stop-loss/take-profit orders, will cancel all pending orders")
	return t.CancelStopOrders(symbol)
}

// CancelAllOrders 取消该币种的所有挂单
func (t *HyperliquidTrader) CancelAllOrders(symbol string) error {
	coin := convertSymbolToHyperliquid(symbol)

	// 获取所有挂单
	openOrders, err := t.exchange.Info().OpenOrders(t.ctx, t.walletAddr)
	if err != nil {
		return fmt.Errorf("failed to retrieve pending orders: %w", err)
	}

	// 取消该币种的所有挂单
	for _, order := range openOrders {
		if order.Coin == coin {
			_, err := t.exchange.Cancel(t.ctx, coin, order.Oid)
			if err != nil {
				log.Printf("  ⚠ Failed to cancel order (oid=%d): %v", order.Oid, err)
			}
		}
	}

	log.Printf("  ✓ All pending orders for %s cancelled", symbol)
	return nil
}

// CancelStopOrders 取消该币种的止盈/止损单（用于调整止盈止损位置）
func (t *HyperliquidTrader) CancelStopOrders(symbol string) error {
	coin := convertSymbolToHyperliquid(symbol)

	// 获取所有挂单
	openOrders, err := t.exchange.Info().OpenOrders(t.ctx, t.walletAddr)
	if err != nil {
		return fmt.Errorf("failed to retrieve pending orders: %w", err)
	}

	// 注意：Hyperliquid SDK 的 OpenOrder 结构不暴露 trigger 字段
	// 因此暂时取消该币种的所有挂单（包括止盈止损单）
	// 这是安全的，因为在设置新的止盈止损之前，应该清理所有旧订单
	canceledCount := 0
	for _, order := range openOrders {
		if order.Coin == coin {
			_, err := t.exchange.Cancel(t.ctx, coin, order.Oid)
			if err != nil {
				log.Printf("  ⚠ Failed to cancel order (oid=%d): %v", order.Oid, err)
				continue
			}
			canceledCount++
		}
	}

	if canceledCount == 0 {
		log.Printf("  ℹ No pending orders to cancel for %s", symbol)
	} else {
		log.Printf("  ✓ %d pending orders for %s cancelled (including take-profit/stop-loss orders)", canceledCount, symbol)
	}

	return nil
}

// GetMarketPrice 获取市场价格
func (t *HyperliquidTrader) GetMarketPrice(symbol string) (float64, error) {
	coin := convertSymbolToHyperliquid(symbol)

	// 获取所有市场价格
	allMids, err := t.exchange.Info().AllMids(t.ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to retrieve price: %w", err)
	}

	// 查找对应币种的价格（allMids是map[string]string）
	if priceStr, ok := allMids[coin]; ok {
		priceFloat, err := strconv.ParseFloat(priceStr, 64)
		if err == nil {
			return priceFloat, nil
		}
		return 0, fmt.Errorf("price format error: %v", err)
	}

	return 0, fmt.Errorf("price not found for %s", symbol)
}

// SetStopLoss 设置止损单
func (t *HyperliquidTrader) SetStopLoss(symbol string, positionSide string, quantity, stopPrice float64) error {
	coin := convertSymbolToHyperliquid(symbol)

	isBuy := positionSide == "SHORT" // 空仓止损=买入，多仓止损=卖出

	// ⚠️ 关键：根据币种精度要求，四舍五入数量
	roundedQuantity := t.roundToSzDecimals(coin, quantity)

	// ⚠️ 关键：价格也需要处理为5位有效数字
	roundedStopPrice := t.roundPriceToSigfigs(stopPrice)

	// 创建止损单（Trigger Order）
	order := hyperliquid.CreateOrderRequest{
		Coin:  coin,
		IsBuy: isBuy,
		Size:  roundedQuantity,  // 使用四舍五入后的数量
		Price: roundedStopPrice, // 使用处理后的价格
		OrderType: hyperliquid.OrderType{
			Trigger: &hyperliquid.TriggerOrderType{
				TriggerPx: roundedStopPrice,
				IsMarket:  true,
				Tpsl:      "sl", // stop loss
			},
		},
		ReduceOnly: true,
	}

	_, err := t.exchange.Order(t.ctx, order, nil)
	if err != nil {
		return fmt.Errorf("failed to set stop-loss: %w", err)
	}

	log.Printf("  Stop-loss price set: %.4f", roundedStopPrice)
	return nil
}

// SetTakeProfit 设置止盈单
func (t *HyperliquidTrader) SetTakeProfit(symbol string, positionSide string, quantity, takeProfitPrice float64) error {
	coin := convertSymbolToHyperliquid(symbol)

	isBuy := positionSide == "SHORT" // 空仓止盈=买入，多仓止盈=卖出

	// ⚠️ 关键：根据币种精度要求，四舍五入数量
	roundedQuantity := t.roundToSzDecimals(coin, quantity)

	// ⚠️ 关键：价格也需要处理为5位有效数字
	roundedTakeProfitPrice := t.roundPriceToSigfigs(takeProfitPrice)

	// 创建止盈单（Trigger Order）
	order := hyperliquid.CreateOrderRequest{
		Coin:  coin,
		IsBuy: isBuy,
		Size:  roundedQuantity,        // 使用四舍五入后的数量
		Price: roundedTakeProfitPrice, // 使用处理后的价格
		OrderType: hyperliquid.OrderType{
			Trigger: &hyperliquid.TriggerOrderType{
				TriggerPx: roundedTakeProfitPrice,
				IsMarket:  true,
				Tpsl:      "tp", // take profit
			},
		},
		ReduceOnly: true,
	}

	_, err := t.exchange.Order(t.ctx, order, nil)
	if err != nil {
		return fmt.Errorf("failed to set take-profit: %w", err)
	}

	log.Printf("  Take-profit price set: %.4f", roundedTakeProfitPrice)
	return nil
}

// FormatQuantity 格式化数量到正确的精度
func (t *HyperliquidTrader) FormatQuantity(symbol string, quantity float64) (string, error) {
	coin := convertSymbolToHyperliquid(symbol)
	szDecimals := t.getSzDecimals(coin)

	// 使用szDecimals格式化数量
	formatStr := fmt.Sprintf("%%.%df", szDecimals)
	return fmt.Sprintf(formatStr, quantity), nil
}

// getSzDecimals 获取币种的数量精度
func (t *HyperliquidTrader) getSzDecimals(coin string) int {
	// ✅ 并发安全：使用读锁保护 meta 字段访问
	t.metaMutex.RLock()
	defer t.metaMutex.RUnlock()

	if t.meta == nil {
		log.Printf("⚠️  meta information is empty, using default precision 4")
		return 4 // 默认精度
	}

	// 在meta.Universe中查找对应的币种
	for _, asset := range t.meta.Universe {
		if asset.Name == coin {
			return asset.SzDecimals
		}
	}

	log.Printf("⚠️  Precision information not found for %s, using default precision 4", coin)
	return 4 // 默认精度
}

// roundToSzDecimals 将数量四舍五入到正确的精度
func (t *HyperliquidTrader) roundToSzDecimals(coin string, quantity float64) float64 {
	szDecimals := t.getSzDecimals(coin)

	// 计算倍数（10^szDecimals）
	multiplier := 1.0
	for i := 0; i < szDecimals; i++ {
		multiplier *= 10.0
	}

	// 四舍五入
	return float64(int(quantity*multiplier+0.5)) / multiplier
}

// roundPriceToSigfigs 将价格四舍五入到5位有效数字
// Hyperliquid要求价格使用5位有效数字（significant figures）
func (t *HyperliquidTrader) roundPriceToSigfigs(price float64) float64 {
	if price == 0 {
		return 0
	}

	const sigfigs = 5 // Hyperliquid标准：5位有效数字

	// 计算价格的数量级
	var magnitude float64
	if price < 0 {
		magnitude = -price
	} else {
		magnitude = price
	}

	// 计算需要的倍数
	multiplier := 1.0
	for magnitude >= 10 {
		magnitude /= 10
		multiplier /= 10
	}
	for magnitude < 1 {
		magnitude *= 10
		multiplier *= 10
	}

	// 应用有效数字精度
	for i := 0; i < sigfigs-1; i++ {
		multiplier *= 10
	}

	// 四舍五入
	rounded := float64(int(price*multiplier+0.5)) / multiplier
	return rounded
}

// convertSymbolToHyperliquid 将标准symbol转换为Hyperliquid格式
// 例如: "BTCUSDT" -> "BTC"
func convertSymbolToHyperliquid(symbol string) string {
	// 去掉USDT后缀
	if len(symbol) > 4 && symbol[len(symbol)-4:] == "USDT" {
		return symbol[:len(symbol)-4]
	}
	return symbol
}

// absFloat 返回浮点数的绝对值
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
