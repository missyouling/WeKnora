// 车队分类的**前端同步种子**（与后端 seed 同源，见 fleet_record_category_seed.go
// 与 knowledge_fleet_document.go 的内置证照定义）。
//
// 为什么需要这份数据（第 34 轮）：
//   分类原先只能通过 `/fleet/categories` 异步拉取。首屏第 1 帧 `menuCategories`
//   是空数组 → `activeSubs` 为空 → 表头只剩复选框与两个系统保留列、KPI 卡片只剩
//   「已收录 / 历史记录」两张，接口回来后才「蹦」出其余卡片与业务列
//   （真机：先 2 张卡停顿一下再出全部、表头先只有 2 列）。
//
// 有了同步种子，第 1 帧就能算出完整卡片组与完整表头；接口返回后用服务端数据
// **整体替换**（服务端是权威：用户可能在设置里改了字段/顺序/启停），
// 所以这里只是「首屏占位」，不会覆盖用户配置。
//
// 维护约定：字段清单必须与后端种子保持一致；`fleetCategorySeed.test.ts` 会校验
// 每个 scope 都有分类、分类字段非空、且每个菜单都能派生 >= 6 个业务列。
import type { FleetCategorySub } from './fleetCategorySubs'

export interface FleetCategorySeed {
  name: string
  subs: FleetCategorySub[]
}

interface FieldOpts {
  dataType?: FleetCategorySub['data_type']
  /** 列表默认展示（后端 def(...)）。 */
  def?: boolean
  /** 筛选栏可见（后端 filt(...)）。 */
  filt?: boolean
}

/** 字段构造：与后端 `def(txt(...))` / `filt(def(...))` 一一对应。 */
function f(name: string, opts: FieldOpts = {}): FleetCategorySub {
  return {
    name,
    enabled: true,
    data_type: opts.dataType || 'text',
    is_default: opts.def === true,
    filterable: opts.filt === true,
  }
}


/** 各 scope 的默认分类（顺序即卡片顺序 / 左侧菜单顺序）。 */
export const FLEET_CATEGORY_SEEDS: Record<string, FleetCategorySeed[]> = {
  // ---- driver ----
  "driver": [
    {
      name: "驾驶证",
      subs: [
        f("驾驶证号", { def: true }), f("司机姓名", { def: true, filt: true }),
        f("性别", { def: true }), f("国籍"),
        f("住址", { def: true }), f("出生日期"),
        f("初次领证日期", { def: true }), f("准驾车型", { def: true }),
        f("有效期起", { def: true }), f("有效期止", { def: true }),
        f("档案编号", { filt: true }), f("发证机关", { def: true }),
        f("驾驶证状态", { def: true }), f("记录"),
      ],
    },
    {
      name: "从业资格证",
      subs: [
        f("司机姓名", { def: true, filt: true }), f("从业资格证号", { def: true }),
        f("住址", { def: true }), f("从业资格类别", { def: true }),
        f("发证机关", { def: true }), f("有效期止", { def: true }),
        f("考核时间", { def: true }), f("证件状态", { def: true, filt: true }),
        f("备注"),
      ],
    },
  ],
  // ---- fleet-cost ----
  "fleet-cost": [
    {
      name: "加油充电",
      subs: [
        f("交易类型", { def: true, filt: true }), f("消费类型"),
        f("卡号"), f("油品/电价类型", { def: true }),
        f("数量", { dataType: "number", def: true }), f("单价", { dataType: "number", def: true }),
        f("百公里油耗/电耗", { dataType: "number" }), f("支付方式"),
        f("税率", { dataType: "number" }), f("税额", { dataType: "number" }),
        f("来源发票号"), f("备注", { dataType: "textarea" }),
        f("车牌号", { def: true, filt: true }), f("金额", { dataType: "number", def: true }),
      ],
    },
    {
      name: "路桥费",
      subs: [
        f("费用类型", { def: true, filt: true }), f("通行起始日期", { dataType: "date", def: true }),
        f("通行截止日期", { dataType: "date", def: true }), f("入口站/起点"),
        f("出口站/终点"), f("停车场名称"),
        f("税率", { dataType: "number" }), f("税额", { dataType: "number", def: true }),
        f("含税金额", { dataType: "number", def: true }), f("停车费", { dataType: "number" }),
        f("支付方式"), f("票据号码"),
        f("来源发票号"), f("备注", { dataType: "textarea" }),
        f("车牌号", { def: true, filt: true }), f("金额", { dataType: "number", def: true }),
      ],
    },
    {
      name: "维修保养费",
      subs: [
        f("维修项目", { def: true, filt: true }), f("维修类别", { def: true }),
        f("维修厂/供应商"), f("工单号"),
        f("工时费", { dataType: "number", def: true }), f("材料费", { dataType: "number", def: true }),
        f("发票号"), f("备注", { dataType: "textarea" }),
        f("车牌号", { def: true, filt: true }), f("金额", { dataType: "number", def: true }),
      ],
    },
    {
      name: "保险理赔",
      subs: [
        f("理赔单号", { def: true, filt: true }), f("关联保单号"),
        f("保险公司"), f("险种"),
        f("出险日期", { dataType: "date", def: true }), f("报案日期", { dataType: "date" }),
        f("司机姓名"), f("事故类型"),
        f("事故地点"), f("事故责任"),
        f("定损金额", { dataType: "number", def: true }), f("赔付日期", { dataType: "date" }),
        f("赔付方式"), f("理赔状态", { def: true }),
        f("免赔金额", { dataType: "number" }), f("自付金额", { dataType: "number" }),
        f("结案日期", { dataType: "date" }), f("备注", { dataType: "textarea" }),
        f("车牌号", { def: true, filt: true }), f("金额", { dataType: "number", def: true }),
      ],
    },
    {
      name: "违章处理",
      subs: [
        f("违章单号", { def: true, filt: true }), f("司机姓名"),
        f("违章类型", { def: true }), f("违章行为"),
        f("违章地点"), f("违章代码"),
        f("扣分", { dataType: "number", def: true }), f("处罚决定书号"),
        f("处理状态", { def: true }), f("处理人"),
        f("处理日期", { dataType: "date" }), f("缴款日期", { dataType: "date" }),
        f("缴款金额", { dataType: "number" }), f("缴款方式"),
        f("是否已缴款"), f("责任人"),
        f("承担方"), f("备注", { dataType: "textarea" }),
        f("车牌号", { def: true, filt: true }), f("金额", { dataType: "number", def: true }),
      ],
    },
    {
      name: "辅材记录",
      subs: [
        f("类型", { def: true, filt: true }), f("数量", { dataType: "number", def: true }),
        f("单价（元）", { dataType: "number", def: true }), f("备注", { dataType: "textarea" }),
        f("车牌号", { def: true, filt: true }), f("金额", { dataType: "number", def: true }),
        f("品牌规格", { def: true }), f("领用人", { def: true }),
        f("领用日期", { dataType: "date", def: true }), f("供应商"),
        f("付款状态"),
      ],
    },
    {
      name: "请车记录",
      subs: [
        f("申请人", { def: true, filt: true }), f("事由", { dataType: "textarea", def: true }),
        f("开始时间", { dataType: "date", def: true }), f("结束时间", { dataType: "date" }),
        f("审批人"), f("备注", { dataType: "textarea" }),
        f("车牌号", { def: true, filt: true }), f("用车地点", { def: true }),
        f("随行人数", { dataType: "number", def: true }), f("审批状态", { def: true }),
      ],
    },
  ],
  // ---- inspection ----
  "inspection": [
    {
      name: "年检管理",
      subs: [
        f("年检类型", { def: true, filt: true }), f("检验周期"),
        f("上次年检日期", { dataType: "date" }), f("下次年检日期", { dataType: "date", def: true }),
        f("到期提醒天数", { dataType: "number" }), f("提醒状态"),
        f("逾期天数", { dataType: "number" }), f("办理状态", { def: true }),
        f("送检日期", { dataType: "date" }), f("送检人"),
        f("检测站"), f("检测结果", { def: true }),
        f("检验有效期", { dataType: "date" }), f("总费用", { dataType: "number" }),
        f("发票号"), f("备注", { dataType: "textarea" }),
        f("车牌号", { def: true, filt: true }), f("金额", { dataType: "number", def: true }),
      ],
    },
  ],
  // ---- maintain ----
  "maintain": [
    {
      name: "一般维修",
      subs: [
        f("工单号", { def: true, filt: true }), f("车牌号", { def: true, filt: true }),
        f("维修日期", { def: true }), f("维修项目", { def: true }),
        f("工时费", { def: true }), f("材料费", { def: true }),
        f("总费用", { def: true }),
      ],
    },
    {
      name: "合格证",
      subs: [
        f("拖修方"), f("车牌号码", { def: true, filt: true }),
        f("厂牌型号", { def: true }), f("发动机型号/编号", { def: true }),
        f("底盘（车身）号", { def: true }), f("维修类别", { def: true }),
        f("合格证类型"), f("维修合同编号", { def: true }),
        f("维修档案编号", { def: true }), f("出厂里程表示值", { dataType: "number", def: true }),
        f("质检员", { def: true }), f("承修方", { def: true }),
        f("承修方许可证号"), f("进厂日期", { dataType: "date", def: true }),
        f("出厂日期", { dataType: "date", def: true }), f("接车人", { def: true }),
        f("接车日期", { dataType: "date", def: true }),
      ],
    },
  ],
  // ---- tire ----
  "tire": [
    {
      name: "轮胎管理",
      subs: [
        f("轮胎编号", { def: true, filt: true }), f("品牌", { def: true }),
        f("规格型号", { def: true }), f("轮胎类型"),
        f("适用轮位"), f("安装轮位"),
        f("安装日期", { dataType: "date", def: true }), f("安装里程", { dataType: "number" }),
        f("当前状态", { def: true }), f("购置日期", { dataType: "date" }),
        f("购置单价", { dataType: "number" }), f("当前里程", { dataType: "number" }),
        f("供应商"), f("付款状态"),
        f("备注", { dataType: "textarea" }), f("车牌号", { def: true, filt: true }),
        f("金额", { dataType: "number", def: true }),
      ],
    },
  ],
  // ---- vehicle ----
  "vehicle": [
    {
      name: "道路运输经营许可证",
      subs: [
        f("许可证号", { def: true, filt: true }), f("业户名称", { def: true, filt: true }),
        f("经营地址", { def: true }), f("经营范围", { def: true }),
        f("发证机关", { def: true }), f("发证日期", { dataType: "date", def: true }),
        f("有效期起", { dataType: "date", def: true }), f("有效期止", { dataType: "date", def: true }),
        f("证件状态", { def: true, filt: true }), f("备注", { dataType: "textarea", def: true }),
      ],
    },
    {
      name: "道路运输证",
      subs: [
        f("道路运输证号", { def: true }), f("业户名称", { def: true, filt: true }),
        f("经营地址", { def: true }), f("车牌号", { def: true, filt: true }),
        f("经营许可证号", { def: true }), f("车辆类型", { def: true }),
        f("吨（座）位", { def: true }), f("经营范围", { def: true }),
        f("车辆尺寸", { def: true }), f("发证日期", { def: true }),
        f("有效期止", { def: true }), f("发证机关", { def: true }),
        f("审验有效期至"), f("技术评定等级", { def: true }),
        f("评定日期"), f("证照状态"),
        f("备注", { def: true }),
      ],
    },
    {
      name: "车辆登记证书",
      subs: [
        f("登记证书编号", { def: true }), f("机动车所有人", { def: true }),
        f("车牌号码", { def: true, filt: true }), f("车辆识别代号", { def: true }),
        f("车辆品牌", { def: true }), f("车辆型号", { def: true }),
        f("车辆类型", { def: true }), f("使用性质", { def: true }),
        f("登记日期", { def: true }), f("发证日期", { def: true }),
        f("身份证明名称"), f("身份证明号码"),
        f("登记机关"), f("车身颜色"),
        f("国产/进口"), f("发动机号"),
        f("发动机型号"), f("燃料种类"),
        f("排量/功率"), f("制造厂名称"),
        f("转向形式"), f("轮胎数"),
        f("轮胎规格"), f("钢板弹簧片数"),
        f("轴距"), f("轴数"),
        f("外廓尺寸"), f("货厢内部尺寸"),
        f("总质量"), f("核定载质量"),
        f("核定载客"), f("驾驶室载客"),
        f("准牵引总质量"), f("车辆获得方式"),
        f("车辆出厂日期"),
      ],
    },
    {
      name: "保险单",
      subs: [
        f("车牌号码", { filt: true }), f("车架号"),
        f("保险起期"), f("保险止期"),
        f("交强险保单号"), f("商业险保单号"),
        f("交强险保费"), f("商业险保费"),
        f("出单机构"), f("投保人"),
        f("保险类型", { def: true }), f("被保险人"),
        f("统一社会信用代码"), f("行驶证车主"),
        f("被保险人名称", { def: true, filt: true }), f("发动机号码"),
        f("厂牌型号"), f("使用性质"),
        f("核定载客"), f("初登日期"),
        f("车船税合计"), f("保单状态", { def: true, filt: true }),
        f("备注", { def: true }),
      ],
    },
    {
      name: "行驶证",
      subs: [
        f("车牌号", { def: true, filt: true }), f("VIN码/车架号", { def: true }),
        f("发动机号", { def: true }), f("品牌型号", { def: true }),
        f("车辆类型", { def: true }), f("使用性质", { def: true }),
        f("注册日期", { def: true }), f("发证日期", { def: true }),
        f("发证机关", { def: true }), f("强制报废日期", { def: true }),
        f("证件状态", { def: true, filt: true }), f("备注"),
      ],
    },
  ],
}

/**
 * 取某 scope 的同步种子分类（浅拷贝，避免调用方改动常量）。
 *
 * 返回结构与 `/fleet/categories` 的 `data[]` 同形（name + subs），
 * 因此可以直接喂给现有的卡片构造与列派生逻辑，无需额外适配层。
 */
export function fleetCategorySeedsOf(scope: string): FleetCategorySeed[] {
  const arr = FLEET_CATEGORY_SEEDS[scope] || []
  return arr.map((c) => ({ name: c.name, subs: c.subs.map((s) => ({ ...s })) }))
}

