export interface Selector {
  type: string
  paths: string[]
}

export interface Filters {
  contains?: string[]
  notContains?: string[]
}

export interface ProductDetection {
  trackStock?: boolean
  trackPrice?: boolean
  minPrice?: number
  maxPrice?: number
}

export interface Monitor {
  name: string
  url: string
  httpHeaders?: Record<string, string[]>
  useChrome: boolean
  interval: number
  selector?: Selector
  filters?: Filters
  ignoreEmpty?: boolean
  productDetection?: ProductDetection
}

export interface PushoverConfig {
  apiToken: string
  userKey: string
}

export interface Notifiers {
  pushover?: PushoverConfig
}

export interface Config {
  monitors: Monitor[]
  notifiers: Notifiers
}

export interface Notification {
  type: 'success' | 'error'
  text: string
}
