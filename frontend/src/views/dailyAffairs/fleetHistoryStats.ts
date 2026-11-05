/**
 * 车队「上传文件」口径（第 39 轮）：列表页 KPI 卡片、在途状态条、历史记录抽屉
 * 三处必须共用同一份判定，否则数字永远对不上。
 *
 * 真机事故（图 1/图 2）：
 *   · 「历史记录」卡片显示 2 份，点开抽屉却是「全部文件 13 / 正在执行 3」；
 *   · 列表上方没有绿色在途状态条，抽屉里却有 3 个文件在解析/提取。
 * 根因是两套口径：
 *   · 卡片取 `fleet_records` 的行数（它是「已收录台账」），抽屉取知识库文件数；
 *   · 状态条的 `pendingFiles` 又按文件元数据里的 scope 过滤，历史文件的
 *     `custom_metadata.scope` 缺失时被整批丢掉 → 条子永远不出现。
 *
 * 现在统一到本文件：
 *   · 文件集合 = 知识库里属于本菜单证照类型的全部文件（抽屉「全部文件」）；
 *   · 在途 = parse 未终态，或 parse 完成但 extract 未终态。
 */

/** 文件所属证照类型（doc_type 优先，历史文件回退 fleet_cert_type）。 */
export const fleetDocTypeOf = (row: any): string => {
  const m = row?.custom_metadata || {}
  return String(m.doc_type || m.fleet_cert_type || '').trim()
}

/**
 * 本菜单的上传文件（抽屉「全部文件」与卡片数字同源）。
 *
 * @param docTypes 该菜单全部分类名；为空表示不过滤类型（记录型菜单没有上传）
 */
export function fleetHistoryFilesOf(files: any[], docTypes?: Set<string> | null): any[] {
  const arr = (Array.isArray(files) ? files : []).filter((r) => !((r?.custom_metadata || {}).fleet_history_hidden))
  if (!docTypes || !docTypes.size) return arr
  return arr.filter((r) => {
    const dt = fleetDocTypeOf(r)
    // 类型未知的历史文件保留（宁可多算一份，也不能让它从抽屉里消失）
    return dt ? docTypes.has(dt) : true
  })
}

/** 在途判定：列表状态条与抽屉「正在执行」必须逐字一致。 */
export function isFleetFileRunning(row: any): boolean {
  const ps = String(row?.parse_status || '')
  const es = String((row?.custom_metadata || {}).extract_status || '')
  // 解析阶段未终态
  if (ps === 'pending' || ps === 'parsing' || ps === 'processing' || ps === 'finalizing') return true
  // 解析完成但提取还没跑完（含「从未提取」→ 待提取）
  if (ps === 'completed' && (!es || es === 'pending' || es === 'processing')) return true
  return false
}

/** 在途文件数（状态条文案 / 历史卡片副文案共用）。 */
export const fleetRunningCountOf = (files: any[]): number =>
  (Array.isArray(files) ? files : []).filter(isFleetFileRunning).length

/** 异常文件数（解析失败 / 提取失败），历史卡片副文案用。 */
export const fleetFailedCountOf = (files: any[]): number =>
  (Array.isArray(files) ? files : []).filter((r) => {
    const es = String((r?.custom_metadata || {}).extract_status || '')
    return r?.parse_status === 'failed' || es === 'failed'
  }).length
