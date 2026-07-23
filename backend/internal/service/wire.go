package service

import (
	"context"
	"database/sql"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/pkg/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// BuildInfo contains build information
type BuildInfo struct {
	Version   string
	BuildType string
}

// ProvidePricingService creates and initializes PricingService
func ProvidePricingService(cfg *config.Config, remoteClient PricingRemoteClient) (*PricingService, error) {
	svc := NewPricingService(cfg, remoteClient)
	if err := svc.Initialize(); err != nil {
		// Pricing service initialization failure should not block startup, use fallback prices
		println("[Service] Warning: Pricing service initialization failed:", err.Error())
	}
	return svc, nil
}

// ProvideUpdateService creates UpdateService with BuildInfo
func ProvideUpdateService(cache UpdateCache, githubClient GitHubReleaseClient, buildInfo BuildInfo) *UpdateService {
	return NewUpdateService(cache, githubClient, buildInfo.Version, buildInfo.BuildType)
}

// ProvideEmailQueueService creates EmailQueueService with default worker count
func ProvideEmailQueueService(emailService *EmailService) *EmailQueueService {
	return NewEmailQueueService(emailService, 3)
}

// ProvideOAuthRefreshAPI creates OAuthRefreshAPI with the default lock TTL.
func ProvideOAuthRefreshAPI(accountRepo AccountRepository, tokenCache GeminiTokenCache) *OAuthRefreshAPI {
	return NewOAuthRefreshAPI(accountRepo, tokenCache)
}

func ProvideWeComOAuthClient(cache WeComOAuthCache) WeComOAuthClient {
	return NewWeComOAuthClient(cache, nil)
}

func ProvideBatchImageModelPricingResolver(resolver *ModelPricingResolver) *BatchImageModelPricingResolver {
	return &BatchImageModelPricingResolver{Resolver: resolver}
}

type redisAdobeVideoTaskCache struct {
	client *redis.Client
}

func (c *redisAdobeVideoTaskCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *redisAdobeVideoTaskCache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return c.client.SetNX(ctx, key, value, ttl).Result()
}

func (c *redisAdobeVideoTaskCache) Get(ctx context.Context, key string) ([]byte, error) {
	value, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, ErrAdobeVideoTaskNotFound
	}
	return value, err
}

func (c *redisAdobeVideoTaskCache) Watch(ctx context.Context, key string, update func([]byte, time.Duration) ([]byte, time.Duration, error)) error {
	return c.client.Watch(ctx, func(tx *redis.Tx) error {
		current, err := tx.Get(ctx, key).Bytes()
		if err == redis.Nil {
			return ErrAdobeVideoTaskNotFound
		}
		if err != nil {
			return err
		}
		remainingTTL, err := tx.PTTL(ctx, key).Result()
		if err != nil || remainingTTL <= 0 {
			return ErrAdobeVideoTaskNotFound
		}
		next, ttl, err := update(current, remainingTTL)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, key, next, ttl)
			return nil
		})
		return err
	}, key)
}

func (c *redisAdobeVideoTaskCache) TryLock(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	return c.client.SetNX(ctx, key, token, ttl).Result()
}

func (c *redisAdobeVideoTaskCache) Unlock(ctx context.Context, key, token string) error {
	const script = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
	return c.client.Eval(ctx, script, []string{key}, token).Err()
}

func NewAdobeVideoTaskStore(client *redis.Client, activeTTL, terminalTTL time.Duration) *RedisAdobeVideoTaskStore {
	var cache adobeVideoTaskCache
	if client != nil {
		cache = &redisAdobeVideoTaskCache{client: client}
	}
	return newAdobeVideoTaskStore(cache, activeTTL, terminalTTL)
}

type redisCursorResponseStore struct {
	client *redis.Client
}

func (s *redisCursorResponseStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *redisCursorResponseStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	return value, err == nil, err
}

func NewCursorGatewayService(httpUpstream HTTPUpstream, proxyRepo ProxyRepository, _ *TLSFingerprintProfileService, redisClient *redis.Client, cfg *config.Config) *CursorGatewayService {
	var responseStore cursorResponseStore
	if redisClient != nil {
		responseStore = &redisCursorResponseStore{client: redisClient}
	}
	return newCursorGatewayService(httpUpstream, proxyRepo, responseStore, cfg)
}

// ProvideAdobeVideoTaskStore applies the configured absolute active/terminal TTLs.
func ProvideAdobeVideoTaskStore(client *redis.Client, cfg *config.Config) AdobeVideoTaskStore {
	activeTTL := 72 * time.Hour
	terminalTTL := 24 * time.Hour
	if cfg != nil {
		if cfg.Adobe.VideoTaskTTLSeconds > 0 {
			activeTTL = time.Duration(cfg.Adobe.VideoTaskTTLSeconds) * time.Second
		}
		if cfg.Adobe.VideoTerminalTTLSeconds > 0 {
			terminalTTL = time.Duration(cfg.Adobe.VideoTerminalTTLSeconds) * time.Second
		}
	}
	return NewAdobeVideoTaskStore(client, activeTTL, terminalTTL)
}

// ProvideAdobeTokenProvider bridges the concrete repositories supplied by Wire
// to the provider's deliberately narrow persistence interfaces.
func ProvideAdobeTokenProvider(accountRepo AccountRepository, proxyRepo ProxyRepository, cfg *config.Config) *AdobeTokenProvider {
	return NewAdobeTokenProvider(accountRepo, proxyRepo, cfg)
}

// ProvideCompositeTokenCacheInvalidator adds Adobe's reload marker to the
// existing shared token-cache invalidator without exposing Adobe secrets.
func ProvideCompositeTokenCacheInvalidator(cache GeminiTokenCache, adobeTokenProvider *AdobeTokenProvider) *CompositeTokenCacheInvalidator {
	invalidator := NewCompositeTokenCacheInvalidator(cache)
	invalidator.SetAdobeTokenProvider(adobeTokenProvider)
	return invalidator
}

func ProvideAccountUsageService(
	accountRepo AccountRepository,
	usageLogRepo UsageLogRepository,
	usageFetcher ClaudeUsageFetcher,
	geminiQuotaService *GeminiQuotaService,
	antigravityQuotaFetcher *AntigravityQuotaFetcher,
	grokQuotaFetcher *GrokQuotaFetcher,
	grokQuotaService *GrokQuotaService,
	openAIQuotaService *OpenAIQuotaService,
	openCodeQuotaService *OpenCodeQuotaService,
	cache *UsageCache,
	identityCache IdentityCache,
	tlsFPProfileService *TLSFingerprintProfileService,
	capacityService *AccountCapacityService,
	adobeTokenProvider *AdobeTokenProvider,
	cursorGatewayService *CursorGatewayService,
	cursorDashboardAuthService *CursorDashboardAuthService,
	kiroUsageService *KiroUsageService,
) *AccountUsageService {
	service := NewAccountUsageService(accountRepo, usageLogRepo, usageFetcher, geminiQuotaService, antigravityQuotaFetcher, grokQuotaFetcher, grokQuotaService, openAIQuotaService, cache, identityCache, tlsFPProfileService)
	service.SetAdobeTokenProvider(adobeTokenProvider)
	service.SetOpenCodeQuotaService(openCodeQuotaService)
	service.SetCursorUsageProber(cursorGatewayService)
	service.SetCursorDashboardFetcher(cursorDashboardAuthService)
	service.SetKiroUsageService(kiroUsageService)
	service.SetAccountCapacityService(capacityService)
	if openAIQuotaService != nil {
		service.agentIdentityWS = openAIQuotaService.agentIdentityWS
	}
	return service
}

func ProvideCursorDashboardMaintenanceService(accountRepo AccountRepository, auth *CursorDashboardAuthService, gateway *CursorGatewayService, cfg *config.Config) *CursorDashboardMaintenanceService {
	svc := NewCursorDashboardMaintenanceService(accountRepo, auth, gateway, cfg)
	svc.Start()
	return svc
}

func ProvideAccountTestService(
	accountRepo AccountRepository,
	geminiTokenProvider *GeminiTokenProvider,
	claudeTokenProvider *ClaudeTokenProvider,
	grokTokenProvider *GrokTokenProvider,
	antigravityGatewayService *AntigravityGatewayService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	tlsFPProfileService *TLSFingerprintProfileService,
	adobeTokenProvider *AdobeTokenProvider,
	cursorGatewayService *CursorGatewayService,
	openCodeGatewayService *OpenCodeGatewayService,
	kiroUsageService *KiroUsageService,
) *AccountTestService {
	service := NewAccountTestService(accountRepo, geminiTokenProvider, claudeTokenProvider, grokTokenProvider, antigravityGatewayService, httpUpstream, cfg, tlsFPProfileService)
	service.SetAdobeTokenProvider(adobeTokenProvider)
	service.SetCursorGatewayService(cursorGatewayService)
	service.SetOpenCodeGatewayService(openCodeGatewayService)
	service.SetKiroUsageService(kiroUsageService)
	return service
}

func ProvideAccountTestServiceWithAgentIdentity(
	accountRepo AccountRepository,
	geminiTokenProvider *GeminiTokenProvider,
	claudeTokenProvider *ClaudeTokenProvider,
	grokTokenProvider *GrokTokenProvider,
	antigravityGatewayService *AntigravityGatewayService,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	tlsFPProfileService *TLSFingerprintProfileService,
	adobeTokenProvider *AdobeTokenProvider,
	cursorGatewayService *CursorGatewayService,
	openCodeGatewayService *OpenCodeGatewayService,
	kiroUsageService *KiroUsageService,
	openAIGatewayService *OpenAIGatewayService,
) *AccountTestService {
	service := ProvideAccountTestService(
		accountRepo,
		geminiTokenProvider,
		claudeTokenProvider,
		grokTokenProvider,
		antigravityGatewayService,
		httpUpstream,
		cfg,
		tlsFPProfileService,
		adobeTokenProvider,
		cursorGatewayService,
		openCodeGatewayService,
		kiroUsageService,
	)
	service.agentIdentityWS = openAIGatewayService
	return service
}

func ProvideBatchImageCleanupService(repo BatchImageRepository, accountRepo AccountRepository, cfg *config.Config) *BatchImageCleanupService {
	svc := NewBatchImageCleanupService(repo, accountRepo, cfg)
	svc.Start()
	return svc
}

// ProvideOpenAIOAuthService creates OpenAIOAuthService with privacy/account enrichment support.
func ProvideOpenAIOAuthService(
	proxyRepo ProxyRepository,
	oauthClient OpenAIOAuthClient,
	privacyClientFactory PrivacyClientFactory,
) *OpenAIOAuthService {
	svc := NewOpenAIOAuthService(proxyRepo, oauthClient)
	svc.SetPrivacyClientFactory(privacyClientFactory)
	return svc
}

// ProvideTokenRefreshService creates and starts TokenRefreshService
func ProvideTokenRefreshService(
	accountRepo AccountRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	antigravityOAuthService *AntigravityOAuthService,
	grokOAuthService *GrokOAuthService,
	kiroOAuthService *KiroOAuthService,
	cacheInvalidator TokenCacheInvalidator,
	schedulerCache SchedulerCache,
	cfg *config.Config,
	tempUnschedCache TempUnschedCache,
	privacyClientFactory PrivacyClientFactory,
	proxyRepo ProxyRepository,
	refreshAPI *OAuthRefreshAPI,
	runtimeBlocker AccountRuntimeBlocker,
) *TokenRefreshService {
	svc := NewTokenRefreshService(accountRepo, oauthService, openaiOAuthService, geminiOAuthService, antigravityOAuthService, cacheInvalidator, schedulerCache, cfg, tempUnschedCache, grokOAuthService)
	// 注册 Kiro 刷新器（通过 setter 注入，避免破坏 Grok 变长参数）
	svc.RegisterKiroRefresher(kiroOAuthService)
	// 注入 OpenAI privacy opt-out 依赖
	svc.SetPrivacyDeps(privacyClientFactory, proxyRepo)
	// 注入统一 OAuth 刷新 API（消除 TokenRefreshService 与 TokenProvider 之间的竞争条件）
	svc.SetRefreshAPI(refreshAPI)
	// 调用侧显式注入后台刷新策略，避免策略漂移
	svc.SetRefreshPolicy(DefaultBackgroundRefreshPolicy())
	svc.SetAccountRuntimeBlocker(runtimeBlocker)
	svc.Start()
	return svc
}

// ProvideClaudeTokenProvider creates ClaudeTokenProvider with OAuthRefreshAPI injection
func ProvideClaudeTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	oauthService *OAuthService,
	refreshAPI *OAuthRefreshAPI,
) *ClaudeTokenProvider {
	p := NewClaudeTokenProvider(accountRepo, tokenCache, oauthService)
	executor := NewClaudeTokenRefresher(oauthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(ClaudeProviderRefreshPolicy())
	return p
}

// ProvideOpenAITokenProvider creates OpenAITokenProvider with OAuthRefreshAPI injection
func ProvideOpenAITokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	openaiOAuthService *OpenAIOAuthService,
	refreshAPI *OAuthRefreshAPI,
) *OpenAITokenProvider {
	p := NewOpenAITokenProvider(accountRepo, tokenCache, openaiOAuthService)
	executor := NewOpenAITokenRefresher(openaiOAuthService, accountRepo)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(OpenAIProviderRefreshPolicy())
	return p
}

// ProvideOpenAIQuotaService wires the OpenAI quota query/reset service.
// It depends on the OpenAI token provider for refreshed access tokens and the
// privacy client factory for the impersonated upstream HTTP client.
func ProvideOpenAIQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *OpenAITokenProvider,
	privacyClientFactory PrivacyClientFactory,
	openAIGatewayService *OpenAIGatewayService,
) *OpenAIQuotaService {
	service := NewOpenAIQuotaService(accountRepo, proxyRepo, tokenProvider, privacyClientFactory)
	service.agentIdentityWS = openAIGatewayService
	return service
}

func ProvideGrokQuotaService(
	accountRepo AccountRepository,
	proxyRepo ProxyRepository,
	tokenProvider *GrokTokenProvider,
	httpUpstream HTTPUpstream,
	cfg *config.Config,
	usageLogRepo UsageLogRepository,
) *GrokQuotaService {
	return NewGrokQuotaService(accountRepo, proxyRepo, tokenProvider, httpUpstream, cfg, usageLogRepo)
}

// ProvideGeminiTokenProvider creates GeminiTokenProvider with OAuthRefreshAPI injection
func ProvideGeminiTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	geminiOAuthService *GeminiOAuthService,
	refreshAPI *OAuthRefreshAPI,
) *GeminiTokenProvider {
	p := NewGeminiTokenProvider(accountRepo, tokenCache, geminiOAuthService)
	executor := NewGeminiTokenRefresher(geminiOAuthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(GeminiProviderRefreshPolicy())
	return p
}

// ProvideAntigravityTokenProvider creates AntigravityTokenProvider with OAuthRefreshAPI injection
func ProvideAntigravityTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	antigravityOAuthService *AntigravityOAuthService,
	refreshAPI *OAuthRefreshAPI,
	tempUnschedCache TempUnschedCache,
) *AntigravityTokenProvider {
	p := NewAntigravityTokenProvider(accountRepo, tokenCache, antigravityOAuthService)
	executor := NewAntigravityTokenRefresher(antigravityOAuthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(AntigravityProviderRefreshPolicy())
	p.SetTempUnschedCache(tempUnschedCache)
	return p
}

// ProvideGrokTokenProvider creates GrokTokenProvider with OAuthRefreshAPI injection.
func ProvideGrokTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	grokOAuthService *GrokOAuthService,
	refreshAPI *OAuthRefreshAPI,
	tempUnschedCache TempUnschedCache,
) *GrokTokenProvider {
	p := NewGrokTokenProvider(accountRepo, tokenCache)
	executor := NewGrokTokenRefresher(grokOAuthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(GrokProviderRefreshPolicy())
	p.SetTempUnschedCache(tempUnschedCache)
	return p
}

// ProvideKiroTokenProvider creates KiroTokenProvider with OAuthRefreshAPI injection.
func ProvideKiroTokenProvider(
	accountRepo AccountRepository,
	tokenCache GeminiTokenCache,
	kiroOAuthService *KiroOAuthService,
	refreshAPI *OAuthRefreshAPI,
	tempUnschedCache TempUnschedCache,
) *KiroTokenProvider {
	p := NewKiroTokenProvider(accountRepo, tokenCache)
	executor := NewKiroTokenRefresher(kiroOAuthService)
	p.SetRefreshAPI(refreshAPI, executor)
	p.SetRefreshPolicy(AntigravityProviderRefreshPolicy())
	p.SetTempUnschedCache(tempUnschedCache)
	return p
}

// ProvideKiroOAuthService creates the Kiro OAuth service.
func ProvideKiroOAuthService(proxyRepo ProxyRepository) *KiroOAuthService {
	return NewKiroOAuthService(proxyRepo)
}

// ProvideKiroUsageService creates the Kiro usage/overage probe service.
func ProvideKiroUsageService(accountRepo AccountRepository, proxyRepo ProxyRepository, tokenProvider *KiroTokenProvider) *KiroUsageService {
	return NewKiroUsageService(accountRepo, proxyRepo, tokenProvider)
}

// ProvideDashboardAggregationService 创建并启动仪表盘聚合服务
func ProvideDashboardAggregationService(repo DashboardAggregationRepository, timingWheel *TimingWheelService, lockCache LeaderLockCache, db *sql.DB, cfg *config.Config) *DashboardAggregationService {
	svc := NewDashboardAggregationService(repo, timingWheel, cfg)
	svc.SetLeaderLock(lockCache, db)
	svc.Start()
	return svc
}

// ProvideUsageCleanupService 创建并启动使用记录清理任务服务
func ProvideUsageCleanupService(repo UsageCleanupRepository, timingWheel *TimingWheelService, dashboardAgg *DashboardAggregationService, cfg *config.Config) *UsageCleanupService {
	svc := NewUsageCleanupService(repo, timingWheel, dashboardAgg, cfg)
	svc.Start()
	return svc
}

// ProvideAccountExpiryService creates and starts AccountExpiryService.
func ProvideAccountExpiryService(accountRepo AccountRepository) *AccountExpiryService {
	svc := NewAccountExpiryService(accountRepo, time.Minute)
	svc.Start()
	return svc
}

// ProvideProxyExpiryService creates and starts ProxyExpiryService.
func ProvideProxyExpiryService(proxyRepo ProxyRepository) *ProxyExpiryService {
	svc := NewProxyExpiryService(proxyRepo, time.Minute)
	svc.Start()
	return svc
}

// ProvideSubscriptionExpiryService creates and starts SubscriptionExpiryService.
func ProvideSubscriptionExpiryService(userSubRepo UserSubscriptionRepository, settingRepo SettingRepository, notificationEmailService *NotificationEmailService, lockCache LeaderLockCache, db *sql.DB) *SubscriptionExpiryService {
	svc := NewSubscriptionExpiryService(userSubRepo, time.Minute)
	svc.SetSettingRepository(settingRepo)
	svc.SetNotificationEmailService(notificationEmailService)
	svc.SetLeaderLock(lockCache, db)
	svc.Start()
	return svc
}

// ProvideTimingWheelService creates and starts TimingWheelService
func ProvideTimingWheelService() (*TimingWheelService, error) {
	svc, err := NewTimingWheelService()
	if err != nil {
		return nil, err
	}
	svc.Start()
	return svc, nil
}

// ProvideDeferredService creates and starts DeferredService
func ProvideDeferredService(accountRepo AccountRepository, timingWheel *TimingWheelService) *DeferredService {
	svc := NewDeferredService(accountRepo, timingWheel, 10*time.Second)
	svc.Start()
	return svc
}

// ProvideConcurrencyService creates ConcurrencyService and starts slot cleanup worker.
func ProvideConcurrencyService(cache ConcurrencyCache, accountRepo AccountRepository, cfg *config.Config) *ConcurrencyService {
	svc := NewConcurrencyService(cache)
	if err := svc.CleanupStaleProcessSlots(context.Background()); err != nil {
		logger.LegacyPrintf("service.concurrency", "Warning: startup cleanup stale process slots failed: %v", err)
	}
	if cfg != nil {
		svc.SetAccountLoadBatchCacheTTL(time.Duration(cfg.Gateway.Scheduling.LoadBatchCacheTTLMS) * time.Millisecond)
		svc.StartSlotCleanupWorker(accountRepo, cfg.Gateway.Scheduling.SlotCleanupInterval)
	}
	return svc
}

// ProvideUserMessageQueueService 创建用户消息串行队列服务并启动清理 worker
func ProvideUserMessageQueueService(cache UserMsgQueueCache, rpmCache RPMCache, cfg *config.Config) *UserMessageQueueService {
	svc := NewUserMessageQueueService(cache, rpmCache, &cfg.Gateway.UserMessageQueue)
	if cfg.Gateway.UserMessageQueue.CleanupIntervalSeconds > 0 {
		svc.StartCleanupWorker(time.Duration(cfg.Gateway.UserMessageQueue.CleanupIntervalSeconds) * time.Second)
	}
	return svc
}

// ProvideSchedulerSnapshotService creates and starts SchedulerSnapshotService.
func ProvideSchedulerSnapshotService(
	cache SchedulerCache,
	outboxRepo SchedulerOutboxRepository,
	accountRepo AccountRepository,
	groupRepo GroupRepository,
	cfg *config.Config,
) *SchedulerSnapshotService {
	svc := NewSchedulerSnapshotService(cache, outboxRepo, accountRepo, groupRepo, cfg)
	svc.Start()
	return svc
}

// ProvideRateLimitService creates RateLimitService with optional dependencies.
func ProvideRateLimitService(
	accountRepo AccountRepository,
	usageRepo UsageLogRepository,
	cfg *config.Config,
	geminiQuotaService *GeminiQuotaService,
	tempUnschedCache TempUnschedCache,
	timeoutCounterCache TimeoutCounterCache,
	openAI403CounterCache OpenAI403CounterCache,
	settingService *SettingService,
	tokenCacheInvalidator TokenCacheInvalidator,
) *RateLimitService {
	svc := NewRateLimitService(accountRepo, usageRepo, cfg, geminiQuotaService, tempUnschedCache)
	svc.SetTimeoutCounterCache(timeoutCounterCache)
	svc.SetOpenAI403CounterCache(openAI403CounterCache)
	svc.SetSettingService(settingService)
	svc.SetTokenCacheInvalidator(tokenCacheInvalidator)
	return svc
}

// ProvideOpsMetricsCollector creates and starts OpsMetricsCollector.
func ProvideOpsMetricsCollector(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	accountRepo AccountRepository,
	concurrencyService *ConcurrencyService,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsMetricsCollector {
	collector := NewOpsMetricsCollector(opsRepo, settingRepo, accountRepo, concurrencyService, db, redisClient, cfg)
	collector.Start()
	return collector
}

// ProvideOpsAggregationService creates and starts OpsAggregationService (hourly/daily pre-aggregation).
func ProvideOpsAggregationService(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsAggregationService {
	svc := NewOpsAggregationService(opsRepo, settingRepo, db, redisClient, cfg)
	svc.Start()
	return svc
}

// ProvideOpsAlertEvaluatorService creates and starts OpsAlertEvaluatorService.
func ProvideOpsAlertEvaluatorService(
	opsService *OpsService,
	opsRepo OpsRepository,
	emailService *EmailService,
	redisClient *redis.Client,
	cfg *config.Config,
	proxyRepo ProxyRepository,
) *OpsAlertEvaluatorService {
	svc := NewOpsAlertEvaluatorService(opsService, opsRepo, emailService, redisClient, cfg, proxyRepo)
	svc.Start()
	return svc
}

// ProvideOpsCleanupService creates and starts OpsCleanupService (cron scheduled).
// channelMonitorSvc 让维护任务（聚合 + 历史/聚合软删）跟随 ops 清理 cron 一起跑，
// 共享 leader lock + heartbeat。
// settingRepo 让 cleanup service 自己读 ops_advanced_settings.data_retention 覆盖 cfg；
// opsService 用来反向注入 cleanup hook，以便 UI 改清理设置时能 Reload cron。
func ProvideOpsCleanupService(
	opsRepo OpsRepository,
	db *sql.DB,
	redisClient *redis.Client,
	cfg *config.Config,
	channelMonitorSvc *ChannelMonitorService,
	settingRepo SettingRepository,
	opsService *OpsService,
) *OpsCleanupService {
	svc := NewOpsCleanupService(opsRepo, db, redisClient, cfg, channelMonitorSvc, settingRepo)
	svc.Start()
	if opsService != nil {
		opsService.SetCleanupReloader(svc)
	}
	return svc
}

func ProvideOpsSystemLogSink(opsRepo OpsRepository) *OpsSystemLogSink {
	sink := NewOpsSystemLogSink(opsRepo)
	sink.Start()
	logger.SetSink(sink)
	return sink
}

// ProvideAuditLogService 创建操作审计日志服务并启动异步写入与保留期清理协程。
// 停止逻辑挂在 cmd/server 的 provideCleanup。
func ProvideAuditLogService(repo AuditLogRepository, settingService *SettingService) *AuditLogService {
	svc := NewAuditLogService(repo, settingService)
	svc.Start()
	return svc
}

func buildIdempotencyConfig(cfg *config.Config) IdempotencyConfig {
	idempotencyCfg := DefaultIdempotencyConfig()
	if cfg != nil {
		if cfg.Idempotency.DefaultTTLSeconds > 0 {
			idempotencyCfg.DefaultTTL = time.Duration(cfg.Idempotency.DefaultTTLSeconds) * time.Second
		}
		if cfg.Idempotency.SystemOperationTTLSeconds > 0 {
			idempotencyCfg.SystemOperationTTL = time.Duration(cfg.Idempotency.SystemOperationTTLSeconds) * time.Second
		}
		if cfg.Idempotency.ProcessingTimeoutSeconds > 0 {
			idempotencyCfg.ProcessingTimeout = time.Duration(cfg.Idempotency.ProcessingTimeoutSeconds) * time.Second
		}
		if cfg.Idempotency.FailedRetryBackoffSeconds > 0 {
			idempotencyCfg.FailedRetryBackoff = time.Duration(cfg.Idempotency.FailedRetryBackoffSeconds) * time.Second
		}
		if cfg.Idempotency.MaxStoredResponseLen > 0 {
			idempotencyCfg.MaxStoredResponseLen = cfg.Idempotency.MaxStoredResponseLen
		}
		idempotencyCfg.ObserveOnly = cfg.Idempotency.ObserveOnly
	}
	return idempotencyCfg
}

func ProvideIdempotencyCoordinator(repo IdempotencyRepository, cfg *config.Config) *IdempotencyCoordinator {
	coordinator := NewIdempotencyCoordinator(repo, buildIdempotencyConfig(cfg))
	SetDefaultIdempotencyCoordinator(coordinator)
	return coordinator
}

func ProvideSystemOperationLockService(repo IdempotencyRepository, cfg *config.Config) *SystemOperationLockService {
	return NewSystemOperationLockService(repo, buildIdempotencyConfig(cfg))
}

func ProvideIdempotencyCleanupService(repo IdempotencyRepository, cfg *config.Config) *IdempotencyCleanupService {
	svc := NewIdempotencyCleanupService(repo, cfg)
	svc.Start()
	return svc
}

// ProvideScheduledTestService creates ScheduledTestService.
func ProvideScheduledTestService(
	planRepo ScheduledTestPlanRepository,
	resultRepo ScheduledTestResultRepository,
) *ScheduledTestService {
	return NewScheduledTestService(planRepo, resultRepo)
}

// ProvideScheduledTestRunnerService creates and starts ScheduledTestRunnerService.
func ProvideScheduledTestRunnerService(
	planRepo ScheduledTestPlanRepository,
	scheduledSvc *ScheduledTestService,
	accountTestSvc *AccountTestService,
	rateLimitSvc *RateLimitService,
	cfg *config.Config,
) *ScheduledTestRunnerService {
	svc := NewScheduledTestRunnerService(planRepo, scheduledSvc, accountTestSvc, rateLimitSvc, cfg)
	svc.Start()
	return svc
}

// ProvideOpsScheduledReportService creates and starts OpsScheduledReportService.
func ProvideOpsScheduledReportService(
	opsService *OpsService,
	userService *UserService,
	emailService *EmailService,
	redisClient *redis.Client,
	cfg *config.Config,
) *OpsScheduledReportService {
	svc := NewOpsScheduledReportService(opsService, userService, emailService, redisClient, cfg)
	svc.Start()
	return svc
}

// ProvideAPIKeyAuthCacheInvalidator 提供 API Key 认证缓存失效能力
func ProvideAPIKeyAuthCacheInvalidator(apiKeyService *APIKeyService) APIKeyAuthCacheInvalidator {
	// Start Pub/Sub subscriber for L1 cache invalidation across instances
	apiKeyService.StartAuthCacheInvalidationSubscriber(context.Background())
	return apiKeyService
}

// ProvideImageStorageSettingService 构造异步生图对象存储的后台设置服务。
//
// config.yaml 里的 image_storage 作为回落：后台从未保存过设置时沿用它，
// 使升级前已通过配置文件开启该功能的部署不被打断。
func ProvideImageStorageSettingService(
	settingRepo SettingRepository,
	encryptor SecretEncryptor,
	backup *BackupService,
	factory ImageStorageFactory,
	cfg *config.Config,
) *ImageStorageSettingService {
	if cfg.ImageStorage.Enabled && !cfg.ImageStorage.Active() {
		// 列出具体缺失的键。若这些键其实已在环境变量里设过，说明它们没被读进来，
		// 请确认 setDefaults 中已为其注册默认值（见 config.setEnvReachableDefaults）。
		logger.L().Warn("image_storage.enabled is true in config but object storage is not fully configured; configure it in the admin UI or complete the config file",
			zap.Strings("missing_keys", cfg.ImageStorage.MissingCredentialKeys()))
	}
	return NewImageStorageSettingService(settingRepo, encryptor, backup, factory, cfg.ImageStorage)
}

// ProvideImageTaskService 构造异步图片任务服务。
//
// 对象存储是异步图片任务的启用前提：仅当开关打开且凭证齐全时功能才可用，否则整体禁用
// （handler 返回 404，不创建任务、不写 Redis），从而避免大 base64 结果撑爆 Redis。
// 启用状态由 settings 服务在运行时解析，因此后台改开关后无需重启即可生效。
func ProvideImageTaskService(store ImageTaskStore, settings *ImageStorageSettingService) *ImageTaskService {
	return NewImageTaskServiceWithResolver(store, settings.Resolver(), defaultImageTaskTTL, defaultImageTaskExecutionTimeout)
}

// ProvideBackupService creates and starts BackupService
func ProvideBackupService(
	settingRepo SettingRepository,
	cfg *config.Config,
	encryptor SecretEncryptor,
	storeFactory BackupObjectStoreFactory,
	dumper DBDumper,
) *BackupService {
	svc := NewBackupService(settingRepo, cfg, encryptor, storeFactory, dumper)
	svc.Start()
	return svc
}

// ProvideOpsService constructs OpsService and wires the SettingService-backed quota
// auto-pause cache sink. Mirrors the SetCleanupReloader pattern: OpsService doesn't
// hold a *SettingService reference, but wire injects a tiny callback so writes to
// ops_advanced_settings immediately propagate into the scheduler hot-path cache.
func ProvideOpsService(
	opsRepo OpsRepository,
	settingRepo SettingRepository,
	cfg *config.Config,
	accountRepo AccountRepository,
	userRepo UserRepository,
	concurrencyService *ConcurrencyService,
	gatewayService *GatewayService,
	openAIGatewayService *OpenAIGatewayService,
	geminiCompatService *GeminiMessagesCompatService,
	antigravityGatewayService *AntigravityGatewayService,
	systemLogSink *OpsSystemLogSink,
	settingService *SettingService,
	authCacheInvalidationWorker *AuthCacheInvalidationWorker,
	apiKeyService *APIKeyService,
) *OpsService {
	svc := NewOpsService(
		opsRepo,
		settingRepo,
		cfg,
		accountRepo,
		userRepo,
		concurrencyService,
		gatewayService,
		openAIGatewayService,
		geminiCompatService,
		antigravityGatewayService,
		systemLogSink,
	)
	if settingService != nil {
		svc.SetOpenAIQuotaAutoPauseSettingsSink(settingService.SetOpenAIQuotaAutoPauseSettings)
		// Optional warm-up so the first scheduled request after process start observes
		// a populated cache rather than zero defaults. Best-effort, sync-bounded.
		settingService.WarmOpenAIQuotaAutoPauseSettings(context.Background())
	}
	svc.authCacheInvalidationWorker = authCacheInvalidationWorker
	svc.apiKeyService = apiKeyService
	svc.StartRuntimeSettingsRefresh(context.Background())
	return svc
}

// ProvideOpsIngressRejectAggregator starts the bounded security aggregation
// runtime and attaches it to OpsService, which is the middleware recorder.
func ProvideOpsIngressRejectAggregator(opsRepo OpsRepository, opsService *OpsService) *OpsIngressRejectAggregator {
	repo, ok := opsRepo.(OpsIngressRejectRepository)
	if !ok {
		return nil
	}
	aggregator := NewOpsIngressRejectAggregator(repo)
	aggregator.Start()
	opsService.SetIngressRejectAggregator(aggregator)
	return aggregator
}

// ProvideSettingService wires SettingService with group reader and proxy repo.
func ProvideSettingService(settingRepo SettingRepository, groupRepo GroupRepository, proxyRepo ProxyRepository, cfg *config.Config) *SettingService {
	svc := NewSettingService(settingRepo, cfg)
	svc.SetDefaultSubscriptionGroupReader(groupRepo)
	svc.SetProxyRepository(proxyRepo)
	if err := svc.LoadForwardedClientIPSettings(context.Background()); err != nil {
		logger.LegacyPrintf("service.setting", "Warning: load forwarded client IP settings failed: %v", err)
	}
	if err := svc.MigrateOpenAIAllowClaudeCodeCodexPluginSetting(context.Background()); err != nil {
		logger.LegacyPrintf("service.setting", "Warning: migrate openai allow Claude Code Codex plugin setting failed: %v", err)
	}
	if err := svc.MigrateCodexBodyFingerprintToSignals(context.Background()); err != nil {
		logger.LegacyPrintf("service.setting", "Warning: migrate codex body fingerprint to signals failed: %v", err)
	}
	antigravity.SetUserAgentVersionResolver(svc.GetAntigravityUserAgentVersion)
	return svc
}

// ProvideBillingCacheService wires BillingCacheService with its RPM dependencies.
func ProvideBillingCacheService(
	cache BillingCache,
	userRepo UserRepository,
	subRepo UserSubscriptionRepository,
	apiKeyRepo APIKeyRepository,
	rpmCache UserRPMCache,
	rateRepo UserGroupRateRepository,
	cfg *config.Config,
	userPlatformQuotaRepo UserPlatformQuotaRepository,
) *BillingCacheService {
	return NewBillingCacheService(cache, userRepo, subRepo, apiKeyRepo, rpmCache, rateRepo, cfg, userPlatformQuotaRepo)
}

// ProvideAPIKeyService wires APIKeyService and connects rate-limit cache invalidation.
func ProvideAPIKeyService(
	apiKeyRepo APIKeyRepository,
	userRepo UserRepository,
	groupRepo GroupRepository,
	userSubRepo UserSubscriptionRepository,
	userGroupRateRepo UserGroupRateRepository,
	cache APIKeyCache,
	cfg *config.Config,
	billingCacheService *BillingCacheService,
	concurrencyService *ConcurrencyService,
) *APIKeyService {
	svc := NewAPIKeyService(apiKeyRepo, userRepo, groupRepo, userSubRepo, userGroupRateRepo, cache, cfg)
	svc.SetRateLimitCacheInvalidator(billingCacheService)
	svc.SetConcurrencyService(concurrencyService)
	return svc
}

// ProviderSet is the Wire provider set for all services
var ProviderSet = wire.NewSet(
	// Core services
	NewAuthService,
	NewUserService,
	ProvideAPIKeyService,
	wire.Bind(new(PlaygroundAPIKeyReader), new(*APIKeyService)),
	wire.Bind(new(PlaygroundModelLister), new(*GatewayService)),
	NewPlaygroundService,
	ProvideAPIKeyAuthCacheInvalidator,
	ProvideAuthCacheInvalidationWorker,
	NewGroupService,
	NewCompositeRouteResolver,
	NewAccountService,
	NewProxyService,
	NewRedeemService,
	NewPromoService,
	NewUsageService,
	NewDashboardService,
	ProvidePricingService,
	NewBillingService,
	ProvideBillingCacheService,
	NewAnnouncementService,
	NewAnnouncementEmailService,
	ProvideAnnouncementEmailDispatchRuntime,
	NewAdminService,
	NewGatewayService,
	NewOpenAIGatewayService,
	ProvideAdobeVideoTaskStore,
	ProvideAdobeTokenProvider,
	NewAdobeFireflyClientFactory,
	NewAdobeFireflyAdapter,
	wire.Bind(new(AdobeVideoUpstream), new(*AdobeFireflyAdapter)),
	NewAdobeVideoService,
	ProvideImageStorageSettingService,
	ProvideImageTaskService,
	ProvideBatchImageModelPricingResolver,
	NewBatchImagePublicService,
	NewBatchImageDownloadService,
	ProvideBatchImageCleanupService,
	ProvideOpenAIImageUploadTempService,
	ProvideBatchImageWorkerRuntime,
	wire.Bind(new(AccountRuntimeBlocker), new(*OpenAIGatewayService)),
	NewOAuthService,
	ProvideWeComOAuthClient,
	NewWeChatPaymentOAuthService,
	ProvideOpenAIOAuthService,
	NewGrokOAuthService,
	wire.Bind(new(GrokOAuthTokenService), new(*GrokOAuthService)),
	NewGeminiOAuthService,
	NewGeminiQuotaService,
	ProvideCompositeTokenCacheInvalidator,
	wire.Bind(new(TokenCacheInvalidator), new(*CompositeTokenCacheInvalidator)),
	NewAntigravityOAuthService,
	ProvideOAuthRefreshAPI,
	ProvideGeminiTokenProvider,
	NewGeminiMessagesCompatService,
	ProvideAntigravityTokenProvider,
	ProvideGrokTokenProvider,
	ProvideKiroTokenProvider,
	ProvideKiroOAuthService,
	ProvideKiroUsageService,
	ProvideOpenAITokenProvider,
	ProvideOpenAIQuotaService,
	ProvideGrokQuotaService,
	ProvideClaudeTokenProvider,
	NewAntigravityGatewayService,
	NewCursorGatewayService,
	NewCursorDashboardAuthService,
	ProvideCursorDashboardMaintenanceService,
	NewKiroGatewayService,
	NewOpenCodeGatewayService,
	NewOpenCodeQuotaService,
	NewAccountCapacityService,
	ProvideRateLimitService,
	ProvideAccountUsageService,
	ProvideGroupPredictedBalanceService,
	wire.Bind(new(GroupPredictedBalanceReader), new(*GroupPredictedBalanceService)),
	ProvideAccountTestServiceWithAgentIdentity,
	ProvideUpstreamBillingProbeService,
	ProvideSettingService,
	NewDataManagementService,
	ProvideBackupService,
	ProvideOpsSystemLogSink,
	ProvideOpsService,
	ProvideOpsIngressRejectAggregator,
	ProvideAuditLogService,
	ProvideOpsMetricsCollector,
	ProvideOpsAggregationService,
	ProvideOpsAlertEvaluatorService,
	ProvideOpsCleanupService,
	ProvideOpsScheduledReportService,
	NewEmailService,
	NewNotificationEmailService,
	ProvideEmailQueueService,
	ProvideQQBotUserLookup,
	NewQQBotService,
	wire.Bind(new(PoolCapacityQQNotifier), new(*QQBotService)),
	ProvidePoolCapacityAlertService,
	ProvidePoolCapacityAlertGatewayBinding,
	NewTurnstileService,
	NewSubscriptionService,
	wire.Bind(new(DefaultSubscriptionAssigner), new(*SubscriptionService)),
	ProvideConcurrencyService,
	ProvideUserMessageQueueService,
	NewUsageRecordWorkerPool,
	ProvideSchedulerSnapshotService,
	NewIdentityService,
	NewCRSSyncService,
	ProvideUpdateService,
	ProvideTokenRefreshService,
	wire.Bind(new(GrokOAuthReconciler), new(*TokenRefreshService)),
	ProvideAccountExpiryService,
	ProvideProxyExpiryService,
	ProvideSubscriptionExpiryService,
	ProvideTimingWheelService,
	ProvideDashboardAggregationService,
	ProvideUsageCleanupService,
	ProvideDeferredService,
	NewAntigravityQuotaFetcher,
	NewGrokQuotaFetcher,
	NewUserAttributeService,
	NewUsageCache,
	NewTotpService,
	NewErrorPassthroughService,
	NewTLSFingerprintProfileService,
	NewDigestSessionStore,
	ProvideIdempotencyCoordinator,
	ProvideSystemOperationLockService,
	ProvideIdempotencyCleanupService,
	ProvideScheduledTestService,
	ProvideScheduledTestRunnerService,
	NewGroupCapacityService,
	NewChannelService,
	NewModelPricingResolver,
	NewContentModerationService,
	NewAffiliateService,
	ProvidePaymentConfigService,
	ProvidePaymentService,
	ProvidePaymentOrderExpiryService,
	ProvideBalanceNotifyService,
	ProvideChannelMonitorService,
	ProvideChannelMonitorRunner,
	NewChannelMonitorRequestTemplateService,
	ProvideUserPlatformQuotaUsageFlusher,
)

// ProvideUserPlatformQuotaUsageFlusher 创建并启动 UserPlatformQuotaUsageFlusher。
func ProvideUserPlatformQuotaUsageFlusher(cfg *config.Config, cache BillingCache, quotaRepo UserPlatformQuotaRepository, tw *TimingWheelService) *UserPlatformQuotaUsageFlusher {
	svc := NewUserPlatformQuotaUsageFlusher(cfg, cache, quotaRepo, tw)
	svc.Start()
	return svc
}

// ProvidePaymentConfigService wraps NewPaymentConfigService to accept the named
// payment.EncryptionKey type instead of raw []byte, avoiding Wire ambiguity.
func ProvidePaymentConfigService(entClient *dbent.Client, settingRepo SettingRepository, key payment.EncryptionKey) *PaymentConfigService {
	return NewPaymentConfigService(entClient, settingRepo, []byte(key))
}

// ProvideBalanceNotifyService creates BalanceNotifyService
func ProvideBalanceNotifyService(emailService *EmailService, settingRepo SettingRepository, accountRepo AccountRepository, notificationEmailService *NotificationEmailService) *BalanceNotifyService {
	svc := NewBalanceNotifyService(emailService, settingRepo, accountRepo)
	svc.SetNotificationEmailService(notificationEmailService)
	return svc
}

// ProvidePaymentService creates PaymentService and attaches notification email delivery.
func ProvidePaymentService(entClient *dbent.Client, registry *payment.Registry, loadBalancer payment.LoadBalancer, redeemService *RedeemService, subscriptionSvc *SubscriptionService, configService *PaymentConfigService, userRepo UserRepository, groupRepo GroupRepository, affiliateService *AffiliateService, notificationEmailService *NotificationEmailService, wechatPaymentOAuthService *WeChatPaymentOAuthService) *PaymentService {
	svc := NewPaymentService(entClient, registry, loadBalancer, redeemService, subscriptionSvc, configService, userRepo, groupRepo, affiliateService)
	svc.SetNotificationEmailService(notificationEmailService)
	svc.SetWeChatPaymentOAuthService(wechatPaymentOAuthService)
	return svc
}

// ProvidePaymentOrderExpiryService creates and starts PaymentOrderExpiryService.
func ProvidePaymentOrderExpiryService(paymentSvc *PaymentService, lockCache LeaderLockCache, db *sql.DB) *PaymentOrderExpiryService {
	svc := NewPaymentOrderExpiryService(paymentSvc, 60*time.Second)
	svc.SetLeaderLock(lockCache, db)
	svc.Start()
	return svc
}

// ProvideChannelMonitorService 创建渠道监控服务（CRUD + RunCheck + 用户视图聚合）。
// 加密器复用 wire 中已注入的 SecretEncryptor（AES-256-GCM）。
func ProvideChannelMonitorService(
	repo ChannelMonitorRepository,
	encryptor SecretEncryptor,
) *ChannelMonitorService {
	return NewChannelMonitorService(repo, encryptor)
}

// ProvideChannelMonitorRunner 创建并启动渠道监控调度器。
// 通过 SetScheduler 注入回 service 后再 Start，确保启动时加载所有 enabled monitor，
// 后续 CRUD 也能即时同步任务表。Runner.Stop 由 cleanup function 调用。
// settingService 用于 runner 每次 fire 读取功能开关。
func ProvideChannelMonitorRunner(svc *ChannelMonitorService, settingService *SettingService) *ChannelMonitorRunner {
	r := NewChannelMonitorRunner(svc, settingService)
	svc.SetScheduler(r)
	r.Start()
	return r
}
