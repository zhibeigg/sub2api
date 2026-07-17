declare module 'weixin-js-sdk' {
  export interface WeixinJSSDK {
    config(config: {
      debug: boolean
      appId: string
      timestamp: number
      nonceStr: string
      signature: string
      jsApiList: string[]
    }): void
    ready(callback: () => void): void
    error(callback: (error: unknown) => void): void
  }

  const wx: WeixinJSSDK
  export default wx
}
