export function splitCharacters(text: string): string[] {
  return Array.from(text)
}

export function resolveStoryIndex(progress: number, total: number): number {
  if (!Number.isFinite(progress) || total <= 1) {
    return 0
  }

  const clamped = Math.min(1, Math.max(0, progress))
  return Math.min(total - 1, Math.floor(clamped * total))
}

export function shouldReducePublicMotion(options: {
  reducedMotion: boolean
  coarsePointer: boolean
  saveData?: boolean
}): boolean {
  return options.reducedMotion || options.coarsePointer || options.saveData === true
}
