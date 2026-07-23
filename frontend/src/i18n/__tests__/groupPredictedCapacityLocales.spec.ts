import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('group predicted capacity locale keys', () => {
  it('uses generic balance/capacity copy and distinguishes account cost from user pricing', () => {
    expect(en.admin.groups.columns.predictedCapacity).toBe('Est. Balance / Capacity')
    expect(zh.admin.groups.columns.predictedCapacity).toBe('预估余额 / 容量')

    expect(en.admin.groups.predictedCapacity).toMatchObject({
      capacity: 'Capacity',
      requests: 'Requests',
      images: 'Images',
      imageUnit: 'images',
      error: 'Load failed',
    })
    expect(zh.admin.groups.predictedCapacity).toMatchObject({
      capacity: '容量',
      requests: '请求',
      images: '图片',
      imageUnit: '张',
      error: '加载失败',
    })

    expect(en.admin.groups.predictedCapacityConfig.unitCost.hint).toContain('not the user-facing sale price')
    expect(en.admin.groups.predictedCapacityConfig.unitCost.hint).toContain('independent')
    expect(zh.admin.groups.predictedCapacityConfig.unitCost.hint).toContain('不是面向用户的销售价格')
    expect(zh.admin.groups.predictedCapacityConfig.unitCost.hint).toContain('不与分组费率或图片售价联动')
  })
})
