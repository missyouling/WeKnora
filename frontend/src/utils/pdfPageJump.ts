/**
 * 浏览器内置 PDF 阅读器定位页码的几何约束。
 *
 * 阅读器默认 fit-page（整页适配）：每页高度≈视口高度，末页（第 N 页）的页顶
 * 永远滚不到视口顶部，`#page=N` 被滚动边界夹紧，页码指示停在 N-1；视口偏矮时
 * 夹紧更严重——6 页票据实测停在 3/6。显式给出数值缩放后，阅读器按真实页偏移
 * 滚动，页码才精准命中（`zoom=page-width` 会被 Chrome 忽略，只能自己换算）。
 */

/** A4 版面在 100% 缩放下的 CSS 像素尺寸（595×842pt，96dpi ≈ 793×1123px）。 */
const A4_WIDTH_AT_100 = 793;
const A4_HEIGHT_AT_100 = 1123;
/** 过小的缩放会让正文不可读，宁可有横向滚动也要保证页码正确。 */
const MIN_ZOOM = 50;
/** 不做超过 100% 的放大，避免扫描件糊成马赛克。 */
const MAX_ZOOM = 100;

/**
 * 根据预览窗格的真实尺寸计算页码跳转用的缩放百分比。
 *
 * 两个约束：
 *   1. 页高 ≥ 窗格高——否则末页页顶不可达，页码再次被夹紧；
 *   2. 页宽 ≤ 窗格宽——等价 fit-width，不引入横向滚动。
 * 两者冲突（窗格特别高）时优先满足 1，页码正确优先于免横滚。
 *
 * @returns 0 表示尺寸未知，调用方应保持浏览器默认缩放。
 */
export function pageJumpZoom(width: number, height: number): number {
  if (!(width > 0) || !(height > 0)) return 0;
  const fitWidth = Math.floor((width / A4_WIDTH_AT_100) * 100);
  const reachable = Math.ceil((height / A4_HEIGHT_AT_100) * 100);
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, fitWidth, reachable));
}

/**
 * 把目标页码（与可选缩放）合并进预览 URL。
 * 无目标页码时原样返回，保持调用方原有的默认阅读体验。
 */
export function withPageFragment(url: string, page: number, zoom: number): string {
  if (!url || !(page > 0)) return url;
  return zoom > 0 ? `${url}#page=${page}&zoom=${zoom}` : `${url}#page=${page}`;
}
