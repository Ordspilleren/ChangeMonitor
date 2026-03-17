export interface Selector {
  type: string
  paths: string[]
}

export interface Filters {
  contains?: string[]
  notContains?: string[]
}

export interface ProductFeature {
  trackStock?: boolean
  trackPrice?: boolean
  minPrice?: number
  maxPrice?: number
}

export interface GenericFeature {
  selector: Selector
  filters?: Filters
  ignoreEmpty?: boolean
}

export interface Monitor {
  name: string
  url: string
  httpHeaders?: Record<string, string[]>
  useChrome: boolean
  interval: number
  generic?: GenericFeature
  product?: ProductFeature
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
