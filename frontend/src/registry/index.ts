// registry 桶文件：导入即注册（副作用导入）。
//
// 新增业务只需：加一个 registry/<scope>.ts 并在此 import 一行。
// 消费方（extractFieldMeta / FleetCertPanel / ExtractRulePanel /
// FleetUploadDialog / FleetUploadHistoryDrawer）零改动即可获得新 scope 的
// 标签、空态、标准字段与正则可配判定。
import './invoice'
import './document'
import './fleet'
import './utility'
import './energy'

export {
  getScope,
  registerScope,
  scopeKeys,
  labelOf,
  isStandardFieldOf,
  isBuiltinFieldOf,
  hasFieldDict,
  canonicalFieldKey,
  type ScopeKey,
  type ScopeDescriptor,
  type FieldMeta,
  type RegexPresetDef,
} from './businessScope'
export { INVOICE_STRUCT_KEYS } from './invoice'
