import { afterEach, describe, expect, it } from 'vitest'
import {
  addPlaygroundAttachments,
  cleanupPlaygroundAttachments,
  createMemoryPlaygroundAttachmentStorage,
  deletePlaygroundAttachment,
  PLAYGROUND_ATTACHMENT_MAX_IMAGE_BYTES,
  PlaygroundAttachmentStorageUnavailableError,
  PlaygroundAttachmentValidationError,
  readPlaygroundAttachment,
  readPlaygroundAttachmentMetadata,
  setPlaygroundAttachmentStorageForTest,
  validatePlaygroundAttachments,
  type PlaygroundAttachmentPayloadStorage
} from '@/utils/playgroundAttachments'

describe('playground attachments', () => {
  afterEach(() => {
    localStorage.clear()
    setPlaygroundAttachmentStorageForTest(null)
  })

  it('accepts supported image/text types and keeps raw payload outside localStorage', async () => {
    setPlaygroundAttachmentStorageForTest(createMemoryPlaygroundAttachmentStorage())
    const files = [
      new File(['hello'], 'notes.md', { type: 'text/markdown' }),
      new File([new Uint8Array([1, 2, 3])], 'pixel.png', { type: 'image/png' })
    ]
    const result = await addPlaygroundAttachments(files)
    expect(result.attachments).toHaveLength(2)
    expect((await readPlaygroundAttachment(result.attachments[0].id))?.data).toBe('hello')
    expect((await readPlaygroundAttachment(result.attachments[1].id))?.data).toMatch(/^data:image\/png;base64,/)
    expect(readPlaygroundAttachmentMetadata()).toHaveLength(2)
    expect(localStorage.getItem('playground_attachment_metadata_v1')).not.toContain('hello')
  })

  it('enforces count, per-file, type, and total limits', () => {
    expect(() => validatePlaygroundAttachments([
      new File(['x'], 'a.exe', { type: 'application/octet-stream' })
    ])).toThrowError(PlaygroundAttachmentValidationError)
    expect(() => validatePlaygroundAttachments(
      Array.from({ length: 5 }, (_, index) => new File(['x'], `${index}.txt`, { type: 'text/plain' }))
    )).toThrowError(expect.objectContaining({ code: 'too_many' }))
    expect(() => validatePlaygroundAttachments([
      { name: 'large.png', type: 'image/png', size: PLAYGROUND_ATTACHMENT_MAX_IMAGE_BYTES + 1 }
    ])).toThrowError(expect.objectContaining({ code: 'file_too_large' }))
  })

  it('supports deletion and stale reference cleanup', async () => {
    setPlaygroundAttachmentStorageForTest(createMemoryPlaygroundAttachmentStorage())
    const { attachments } = await addPlaygroundAttachments([
      new File(['a'], 'a.txt', { type: 'text/plain' }),
      new File(['b'], 'b.txt', { type: 'text/plain' })
    ])
    await deletePlaygroundAttachment(attachments[0].id)
    expect(await readPlaygroundAttachment(attachments[0].id)).toBeNull()
    await cleanupPlaygroundAttachments([])
    expect(await readPlaygroundAttachment(attachments[1].id)).toBeNull()
    expect(readPlaygroundAttachmentMetadata()).toEqual([])
  })

  it('surfaces an explicit error when IndexedDB storage is unavailable', async () => {
    const unavailable: PlaygroundAttachmentPayloadStorage = {
      async put() { throw new PlaygroundAttachmentStorageUnavailableError() },
      async get() { throw new PlaygroundAttachmentStorageUnavailableError() },
      async delete() {}, async deleteMany() {}, async clear() {}
    }
    setPlaygroundAttachmentStorageForTest(unavailable)
    await expect(addPlaygroundAttachments([new File(['x'], 'x.txt', { type: 'text/plain' })]))
      .rejects.toBeInstanceOf(PlaygroundAttachmentStorageUnavailableError)
  })
})
