/**
 * Resolve a group (platform + name) to a brand identity for display:
 *  - `keyword`: a model-like string that <ModelIcon> already recognizes, so we
 *    reuse the existing 30+ brand logos (Claude / OpenAI / DeepSeek / Kimi /
 *    GLM / MiMo / Qwen / MiniMax ...).
 *  - `colorClass`: a Tailwind text color class matching the brand, used to tint
 *    the group name so the list reads like the colorful brand badges design.
 *
 * Name keywords take priority over platform so a group named "DeepSeek 号池"
 * under an openai-compatible platform still shows the DeepSeek brand.
 */

export interface GroupBrand {
  /** Passed to <ModelIcon :model="keyword" /> — must be a keyword ModelIcon maps. */
  keyword: string
  /** Tailwind text color class for the brand (light + dark). */
  colorClass: string
  /** Normalized brand key for grouping/filtering (e.g. deepseek, qwen, claude). */
  brand: string
  /** Human-readable brand/vendor display name (Anthropic / DeepSeek / 通义千问 ...). */
  label: string
}

// Brand key → human-readable vendor label used by the model marketplace provider filter.
export const BRAND_LABEL: Record<string, string> = {
  claude: 'Anthropic',
  openai: 'OpenAI',
  gemini: 'Gemini',
  grok: 'Grok',
  adobe: 'Adobe',
  opencode: 'OpenCode Go',
  deepseek: 'DeepSeek',
  moonshot: 'Kimi',
  zhipu: '智谱',
  qwen: '通义千问',
  minimax: 'MiniMax',
  mimo: 'MiMo',
  wenxin: '文心',
  spark: '星火',
  hunyuan: '混元',
  doubao: '豆包',
}

// Brand keyword → text color class. Keys align with ModelIcon's iconKey output
// via the intermediate keyword (see KEYWORD_FOR_ICON below).
const BRAND_COLOR: Record<string, string> = {
  claude: 'text-orange-600 dark:text-orange-400',
  openai: 'text-emerald-600 dark:text-emerald-400',
  gemini: 'text-blue-600 dark:text-blue-400',
  grok: 'text-zinc-800 dark:text-zinc-200',
  adobe: 'text-red-700 dark:text-red-400',
  opencode: 'text-teal-700 dark:text-teal-300',
  deepseek: 'text-[#4D6BFE] dark:text-[#7f95ff]',
  moonshot: 'text-zinc-800 dark:text-zinc-100', // Kimi
  zhipu: 'text-[#3859FF] dark:text-[#7c90ff]', // GLM
  qwen: 'text-[#615EFF] dark:text-[#9a98ff]', // 通义千问
  minimax: 'text-[#F23F5D] dark:text-[#ff7d92]',
  mimo: 'text-[#FF6900] dark:text-[#ff9448]', // 小米 MiMo
  wenxin: 'text-[#167ADF] dark:text-[#5fa8ef]', // 文心
  spark: 'text-[#0070F0] dark:text-[#5aa6ff]', // 讯飞星火
  hunyuan: 'text-[#0053E0] dark:text-[#5b93ff]', // 腾讯混元
  doubao: 'text-[#1C64F2] dark:text-[#6b9bff]', // 豆包
}
const DEFAULT_COLOR = 'text-gray-600 dark:text-gray-300'

// Ordered name-keyword rules (checked before platform fallback). Each entry maps
// a set of case-insensitive substrings to a brand keyword understood by ModelIcon.
const NAME_RULES: { match: string[]; keyword: string }[] = [
  { match: ['claude'], keyword: 'claude' },
  { match: ['codex', 'gpt', 'chatgpt', 'o1', 'o3', 'o4'], keyword: 'gpt' },
  { match: ['deepseek'], keyword: 'deepseek' },
  { match: ['kimi', 'moonshot'], keyword: 'kimi' },
  { match: ['glm', 'chatglm', 'zhipu', '智谱'], keyword: 'glm' },
  { match: ['qwen', 'qwq', '通义', '千问'], keyword: 'qwen' },
  { match: ['minimax', 'abab'], keyword: 'minimax' },
  { match: ['mimo', '小米'], keyword: 'mimo' },
  { match: ['gemini', 'gemma'], keyword: 'gemini' },
  { match: ['grok'], keyword: 'grok' },
  { match: ['adobe', 'firefly', 'nano-banana', 'veo3', 'sora'], keyword: 'adobe' },
  { match: ['opencode go', 'opencode'], keyword: 'opencode' },
  { match: ['ernie', 'wenxin', '文心'], keyword: 'ernie' },
  { match: ['spark', '星火'], keyword: 'spark' },
  { match: ['hunyuan', '混元'], keyword: 'hunyuan' },
  { match: ['doubao', '豆包'], keyword: 'doubao' },
]

// Platform → brand keyword fallback when the name has no recognizable brand.
const PLATFORM_KEYWORD: Record<string, string> = {
  anthropic: 'claude',
  openai: 'gpt',
  gemini: 'gemini',
  grok: 'grok',
  adobe: 'adobe',
  antigravity: 'gemini',
  opencode: 'opencode',
}

// Map the resolved keyword to the icon family used for coloring. Keyword strings
// are what we feed ModelIcon; several keywords collapse to the same brand color.
const KEYWORD_TO_BRAND: Record<string, string> = {
  claude: 'claude',
  gpt: 'openai',
  gemini: 'gemini',
  grok: 'grok',
  adobe: 'adobe',
  opencode: 'opencode',
  deepseek: 'deepseek',
  kimi: 'moonshot',
  glm: 'zhipu',
  qwen: 'qwen',
  minimax: 'minimax',
  mimo: 'mimo',
  ernie: 'wenxin',
  spark: 'spark',
  hunyuan: 'hunyuan',
  doubao: 'doubao',
}

export function resolveGroupBrand(platform: string, name: string): GroupBrand {
  const lower = (name || '').toLowerCase()
  let keyword: string | null = null

  for (const rule of NAME_RULES) {
    if (rule.match.some((m) => lower.includes(m.toLowerCase()))) {
      keyword = rule.keyword
      break
    }
  }

  if (!keyword) {
    keyword = PLATFORM_KEYWORD[platform] || 'gpt'
  }

  const brand = KEYWORD_TO_BRAND[keyword] || keyword
  return {
    keyword,
    colorClass: BRAND_COLOR[brand] || DEFAULT_COLOR,
    brand,
    label: BRAND_LABEL[brand] || keyword,
  }
}
