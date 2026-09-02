package providers

import "strings"

// ProviderAliasMap maps short aliases to canonical provider IDs.
var ProviderAliasMap = map[string]string{
	"aai":            "assemblyai",
	"ag":             "antigravity",
	"ali":            "alicode",
	"ali-tp":         "alitp-intl",
	"alii":           "alicode-intl",
	"alitp":          "alitp-intl",
	"ant":            "anthropic",
	"ark":            "volcengine-ark",
	"az":             "azure",
	"bb":             "blackbox",
	"bfl":            "black-forest-labs",
	"bpm":            "byteplus",
	"brave":          "brave-search",
	"cb":             "cerebras",
	"cbai":           "codebuddy-intl",
	"cc":             "claude",
	"cd":             "codebuddy-cn",
	"cf":             "cloudflare-ai",
	"ch":             "chutes",
	"cl":             "cline",
	"cmc":            "commandcode",
	"cp":             "clinepass",
	"cu":             "cursor",
	"cx":             "codex",
	"dg":             "deepgram",
	"ds":             "deepseek",
	"el":             "elevenlabs",
	"fal":            "fal-ai",
	"fish":           "fish-audio",
	"fl":             "featherless",
	"fw":             "fireworks",
	"gb":             "grok-cli",
	"gc":             "gemini-cli",
	"gcli":           "grok-cli",
	"gh":             "github",
	"gl":             "gitlab",
	"glmcn":          "glm-cn",
	"gpse":           "google-pse",
	"gq":             "groq",
	"grok-build":     "grok-cli",
	"gw":             "grok-web",
	"hf":             "huggingface",
	"hyp":            "hyperbolic",
	"if":             "iflow",
	"jina":           "jina-ai",
	"kc":             "kilocode",
	"km":             "kimi",
	"kr":             "kiro",
	"mimo":           "xiaomi-mimo",
	"mm":             "minimax",
	"mmf":            "mimo-free",
	"nb":             "nanobanana",
	"ne":             "nebius",
	"nv":             "nvidia",
	"oa":             "openai",
	"oc":             "opencode",
	"ocg":            "opencode-go",
	"or":             "openrouter",
	"pa":             "perplexity-agent",
	"polly":          "aws-polly",
	"pplx":           "perplexity",
	"pplx-agent":     "perplexity-agent",
	"pplx-responses": "perplexity-agent",
	"pw":             "perplexity-web",
	"qd":             "qoder",
	"runway":         "runwayml",
	"stability":      "stability-ai",
	"tg":             "together",
	"vali":           "volcengine-ark",
	"vercel":         "vercel-ai-gateway",
	"vn":             "venice",
	"xmtp":           "xiaomi-tokenplan",
	"af":             "api-airforce",
	"bzl":            "bazaarlink",
	"bm":             "bluesminds",
	"dv":             "devin-cli",
	"hunyuan":        "tencent",
	"kgw":            "kilo-gateway",
	"qianfan":        "baidu",
	"samba":          "sambanova",
	"tr":             "trae",
	"ws":             "windsurf",
	"zd":             "zed",
}

// ResolveAlias returns the canonical provider ID for an alias, or the alias itself if not found.
func ResolveAlias(alias string) string {
	if canonical, ok := ProviderAliasMap[alias]; ok {
		return canonical
	}
	return alias
}

// CanonicalDefaultAliasMap maps canonical provider IDs to their primary default alias.
var CanonicalDefaultAliasMap = map[string]string{
	"opencode":         "oc",
	"opencode-go":      "ocg",
	"antigravity":      "ag",
	"codex":            "cx",
	"github":           "gh",
	"claude":           "cc",
	"kiro":             "kr",
	"qoder":            "qd",
	"mimo-free":        "mmf",
	"xiaomi-mimo":      "mimo",
}

// GetDefaultProviderAlias returns the default routing prefix alias for a canonical provider.
// If the canonical provider has a designated default alias, that alias is returned.
// Otherwise, it returns the canonical provider ID itself.
func GetDefaultProviderAlias(canonical string) string {
	cLower := strings.ToLower(strings.TrimSpace(canonical))
	if alias, ok := CanonicalDefaultAliasMap[cLower]; ok {
		return alias
	}
	return cLower
}
