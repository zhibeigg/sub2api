import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('risk control locale copy', () => {
  it('describes worker runtime as audit and pre-block record processing', () => {
    expect(zh.admin.riskControl.workerStatusHint).toContain('前置拦截记录任务')
    expect(zh.admin.riskControl.workerStatusHint).not.toContain('异步观察任务')
    expect(en.admin.riskControl.workerStatusHint).toContain('pre-block record tasks')
    expect(en.admin.riskControl.workerStatusHint).not.toContain('observation tasks')
  })

  it('keeps pre-block audit key summary aware of async worker load', () => {
    expect(zh.admin.riskControl.preBlockAPIKeyLoadSummary).toContain('worker：{workerActive} / {workerTotal}')
    expect(en.admin.riskControl.preBlockAPIKeyLoadSummary).toContain('worker: {workerActive} / {workerTotal}')
  })

  it('does not describe pre-block audit key polling as bypassing the worker pool', () => {
    expect(zh.admin.riskControl.preBlockAPIKeyLoadHint).toBe('同步前置拦截直接轮询可用审核 Key。')
    expect(zh.admin.riskControl.preBlockAPIKeyLoadHint).not.toContain('Worker 池')
    expect(en.admin.riskControl.preBlockAPIKeyLoadHint).not.toContain('worker pool')
  })

  it('documents Cyber Abuse defaults, blocking behavior, research context, and legal limits', () => {
    expect(zh.admin.riskControl.cyberAbuseDefaultOffNotice).toContain('默认关闭')
    expect(zh.admin.riskControl.cyberAbuseHitBehaviorNotice).toContain('终止当前请求')
    expect(zh.admin.riskControl.cyberAbuseResearchContextNotice).toContain('合法、获授权')
    expect(zh.admin.riskControl.cyberAbuseLegalNotice).toContain('不替代法律判断')

    expect(en.admin.riskControl.cyberAbuseDefaultOffNotice).toContain('off by default')
    expect(en.admin.riskControl.cyberAbuseHitBehaviorNotice).toContain('terminates the current request')
    expect(en.admin.riskControl.cyberAbuseResearchContextNotice).toContain('authorized security research')
    expect(en.admin.riskControl.cyberAbuseLegalNotice).toContain('do not replace legal judgment')
  })

  it('keeps Cyber Abuse test and log labels available in both locales', () => {
    expect(zh.admin.riskControl.cyberAbuseTestHint).toContain('不写入日志')
    expect(en.admin.riskControl.cyberAbuseTestHint).toContain('without writing logs')
    expect(zh.admin.riskControl.policySource).toBeTruthy()
    expect(en.admin.riskControl.policySource).toBeTruthy()
    expect(zh.admin.riskControl.action.cyberAbuseBlock).toContain('阻断')
    expect(en.admin.riskControl.action.cyberPolicy).toContain('blocked')
  })
})
