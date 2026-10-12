// @ts-nocheck
<script setup lang="ts">
import { indexEmbeddedSourceImages } from '@/utils/sourceImage';
import { ref, shallowRef, watch, onMounted, onUnmounted, nextTick, defineAsyncComponent } from 'vue';
import { previewKnowledgeFile, getKnowledgePreviewUrl } from '@/api/knowledge-base/index';
import { previewTemporaryAttachment } from '@/api/chat/temporary-attachments';
import { downloadArtifact } from '@/api/chat';
import hljs from 'highlight.js';
import 'highlight.js/styles/github.css';
import 'katex/dist/katex.min.css';
import { useI18n } from 'vue-i18n';
import { sanitizeHTML, sanitizeMarkdownHTML } from '@/utils/security';
import { preparePptxPreview, isCompletePptxRender } from '@/utils/pptxPreview';
import { findMarkdownSourceRange } from '@/utils/markdownSourceLocate';
import { renderDocumentPreviewMarkdown } from '@/utils/documentPreviewMarkdown';
import { buildHtmlPreview } from '@/utils/htmlPreview';
import { openMermaidFullscreen } from '@/utils/mermaidViewer';
import { renderMermaidToSvg } from '@/utils/mermaidShared';
import {
  FILE_PREVIEW_SNIFF_BYTES,
  getHighlightLang as resolveHighlightLang,
  getPreviewMimeType,
  prettyPrintJson,
  resolveFilePreviewExt,
  resolvePreviewKind,
  shouldPrettyPrintJson,
  sniffPreview,
  isValidUTF8,
  type FilePreviewKind,
} from '@/utils/filePreview';
import {
  findInText,
  sourceQuoteText,
  type SourceLocateRequest,
} from '@/utils/sourceLocator';
import {
  buildTextIndex,
  findTextRange,
  highlightElements,
  highlightRanges,
  rangeForOffsets,
  scrollRectIntoContainer,
} from '@/utils/sourceLocatorDom';
import { renderEpubPreview } from '@/utils/epubPreview';


const VueOfficePptx = defineAsyncComponent(() => import('@vue-office/pptx'));
const PdfSourceViewer = defineAsyncComponent(() => import('@/components/source-preview/PdfSourceViewer.vue'));

const { t } = useI18n();

const props = defineProps<{
  sourceBlob?: Blob;
  knowledgeId?: string;
  sessionId?: string;
  attachmentId?: string;
  messageId?: string;
  artifactIndex?: number;
  fileType: string;
  fileName: string;
  active: boolean;
  fillHeight?: boolean;
  /**
   * 多页文档（如同一 PDF 拆出的多张发票）中的目标页码（1-based）。
   * 直连预览时与基础 URL 合并为 `url#page=N` 后一次性赋值给 iframe，
   * 避免 URL 与页码分步赋值引发的二次重载；同文件页码切换走浏览器
   * fragment 导航，不重新下载文件。
   */
  page?: number;
  /**
   * Source mode renders PDFs with pdf.js instead of the browser viewer so a
   * citation can be scrolled to and highlighted.
   */
  sourceMode?: boolean;
  /** Where to scroll and what to highlight once the file is rendered. */
  locate?: SourceLocateRequest | null;
}>();

export type SourceLocateResult = { found: boolean; precise: boolean; granularity?: string; reason?: string };

const emit = defineEmits<{
  located: [result: SourceLocateResult];
}>();

const loading = ref(false);
/** 直连基础 URL 模块级缓存（knowledgeId -> {url, ts}）：
 *  跨抽屉打开/组件重建后复用同一预签名 URL，让浏览器 HTTP 缓存命中，
 *  避免每次打开详情都重新签名并整份下载大 PDF（表现为“加载两次/闪烁”）。
 *  预签名有效期为 1h，留 10 分钟余量后失效重取。 */
const directBaseUrlCache = new Map<string, { url: string; ts: number }>();
const DIRECT_URL_TTL_MS = 50 * 60 * 1000;
const DIRECT_URL_CACHE_MAX = 20;

function cachedDirectUrl(knowledgeId: string): string {
  const hit = directBaseUrlCache.get(knowledgeId);
  if (hit && Date.now() - hit.ts < DIRECT_URL_TTL_MS) return hit.url;
  return '';
}

function setDirectUrlCache(knowledgeId: string, url: string) {
  if (directBaseUrlCache.size >= DIRECT_URL_CACHE_MAX) {
    const oldest = directBaseUrlCache.keys().next().value;
    if (oldest !== undefined) directBaseUrlCache.delete(oldest);
  }
  directBaseUrlCache.set(knowledgeId, { url, ts: Date.now() });
}

/** 直连 PDF 已赋值 src、等待 iframe load 的遮罩期（防止切换文件时白屏/二次加载）。 */
const iframePending = ref(false);
const error = ref('');
const previewType = ref<FilePreviewKind>('unsupported');
const blobUrl = ref('');
/** 直连预览 URL（COS 预签名 / local /r/ 授权），浏览器原生支持 Range 渐进加载。 */
const directUrl = ref('');
const textContent = ref('');
const highlightedCode = ref('');
const markdownHtml = ref('');
const excelHtml = ref('');
const mermaidSvg = ref('');
const htmlViewMode = ref<'render' | 'source'>('render');
const pptxData = shallowRef<ArrayBuffer | null>(null);
const pdfData = shallowRef<ArrayBuffer | null>(null);
const epubHtml = ref('');
let epubObjectUrls: string[] = [];
let excelSheetNames: string[] = [];
let previewExt = '';
let pptxSlideCount = 0;
function onPptxRendered(result: unknown) {
  if (!isCompletePptxRender(result, pptxSlideCount)) {
    error.value = t('preview.loadFailed');
    return;
  }
  scheduleLocate();
}
const docxContainer = ref<HTMLElement | null>(null);
const imageNaturalWidth = ref(0);
const imageNaturalHeight = ref(0);
let loadedForId = '';
/** 正在加载中的 sourceKey 及其 Promise：防止并发 loadPreview 重复请求签名 URL
 *  导致 iframe src 被设置两次（第二次会清空第一次的内容，造成闪烁）。
 *  典型触发：详情抽屉 openDetail 先设 currentRow，getKnowledgeDetails 返回后又设一次，
 *  若第二次发生在第一次的 getKnowledgePreviewUrl 等待期间，两次都会拿到不同的签名 URL。 */
let loadingForKey = '';
let loadingPromise: Promise<void> | null = null;

const previewRoot = ref<HTMLElement | null>(null);
const previewContent = ref<HTMLElement | null>(null);

function focusPreviewContent() {
  const root = previewRoot.value;
  if (!props.active || !root?.isConnected) return;
  // Focus the actual scrolling element so the browser handles arrows, Space,
  // PageUp/Down and Home/End, including native iframe and media controls.
  const target = previewContent.value || docxContainer.value || root;
  target.focus({ preventScroll: true });
}

watch(
  () => [props.active, previewRoot.value, getPreviewSourceKey(), props.sourceBlob, htmlViewMode.value],
  async () => {
    if (!props.active) return;
    const previousFocus = document.activeElement;
    await nextTick();
    // Do not override a user who moved to another control while rendering.
    if (document.activeElement !== previousFocus && document.activeElement !== document.body) return;
    focusPreviewContent();
  },
  { flush: 'post' },
);

watch(
  () => [previewContent.value, docxContainer.value],
  () => {
    // While downloading, focus rests on the preview root. Transfer it only
    // if it is still there when the content arrives (never steal input focus).
    if (document.activeElement === previewRoot.value) focusPreviewContent();
  },
  { flush: 'post' },
);

function onPreviewFrameLoad() {
  // 直连 PDF 首次/跨文件加载完成：撤掉遮罩层，避免 loading 状态长期盖住内容。
  if (iframePending.value) {
    iframePending.value = false;
  }
  if (document.activeElement === previewContent.value || document.activeElement === previewRoot.value) {
    focusPreviewContent();
  }
}

function ensureBlobType(blob: Blob, ft: string): Blob {
  const expected = getPreviewMimeType(ft);
  if (blob.type === expected) return blob;
  return new Blob([blob], { type: expected });
}

function getHighlightLang(ft: string): string {
  return resolveHighlightLang(ft);
}

async function renderDocx(blob: Blob) {
  const { renderAsync } = await import('docx-preview');
  if (docxContainer.value) {
    docxContainer.value.innerHTML = '';
    await renderAsync(blob, docxContainer.value, undefined, {
      className: 'docx-preview-wrapper',
      inWrapper: true,
      ignoreWidth: false,
      ignoreHeight: false,
      ignoreFonts: false,
      breakPages: true,
      ignoreLastRenderedPageBreak: true,
      experimental: false,
      trimXmlDeclaration: true,
      useBase64URL: true,
    });
    if (docxContainer.value) await indexEmbeddedSourceImages(docxContainer.value);
  }
}

function decodeCSVBlob(arrayBuffer: ArrayBuffer): string {
  const bytes = new Uint8Array(arrayBuffer);
  if (bytes[0] === 0xEF && bytes[1] === 0xBB && bytes[2] === 0xBF) {
    return new TextDecoder('utf-8').decode(bytes);
  }
  if (isValidUTF8(bytes)) {
    return new TextDecoder('utf-8').decode(bytes);
  }
  return new TextDecoder('gbk').decode(bytes);
}

async function renderExcel(blob: Blob, fileType?: string) {
  const XLSX = await import('xlsx');
  const arrayBuffer = await blob.arrayBuffer();

  let workbook;
  const lowerType = fileType?.toLowerCase();
  if (lowerType === 'csv') {
    const csvText = decodeCSVBlob(arrayBuffer);
    workbook = XLSX.read(csvText, { type: 'string' });
  } else if (lowerType === 'tsv' || lowerType === 'tab') {
    const tsvText = decodeCSVBlob(arrayBuffer);
    workbook = XLSX.read(tsvText, { type: 'string', FS: '\t' });
  } else {
    workbook = XLSX.read(arrayBuffer, { type: 'array' });
  }

  excelSheetNames = [...workbook.SheetNames];
  let html = '';
  workbook.SheetNames.forEach((name, sheetIdx) => {
    const sheet = workbook.Sheets[name];
    const sheetHtml = XLSX.utils.sheet_to_html(sheet, { id: `sheet-${sheetIdx}` });
    html += `<div class="excel-sheet">`;
    if (workbook.SheetNames.length > 1) {
      html += `<div class="excel-sheet-name">${name}</div>`;
    }
    html += sheetHtml;
    html += `</div>`;
  });
  excelHtml.value = sanitizeHTML(html);
}

async function renderText(blob: Blob, fileType: string) {
  let text = await blob.text();
  if (shouldPrettyPrintJson(fileType)) {
    text = prettyPrintJson(text);
  }
  textContent.value = text;

  const lang = getHighlightLang(fileType);
  if (lang && hljs.getLanguage(lang)) {
    try {
      highlightedCode.value = hljs.highlight(text, { language: lang }).value;
      return;
    } catch { /* fallthrough */ }
  }
  const auto = hljs.highlightAuto(text);
  highlightedCode.value = auto.value;
}

async function renderMarkdown(blob: Blob) {
  const text = await blob.text();

  // 校验文本内容是否有效
  if (!text || typeof text !== 'string') {
    markdownHtml.value = '<p style="color: var(--td-text-color-disabled); text-align: center; padding: 20px;">文档内容为空</p>';
    return;
  }

  markdownHtml.value = renderDocumentPreviewMarkdown(text);
}

function onImageLoad(e: Event) {
  const img = e.target as HTMLImageElement;
  imageNaturalWidth.value = img.naturalWidth;
  imageNaturalHeight.value = img.naturalHeight;
}

function getPreviewSourceKey(): string {
  if (props.sourceBlob) {
    return `resource-blob:${props.fileName}:${props.fileType}:${props.sourceBlob.size}:${props.sourceBlob.type}`;
  }
  // 同一 knowledge 文件的多页发票共享 knowledgeId：把页码纳入缓存键，
  // 保证切换同文件不同页时能触发重定位，而不是命中缓存直接跳过。
  if (props.knowledgeId) return `knowledge:${props.knowledgeId}${props.page ? `#p${props.page}` : ''}`;
  if (props.sessionId && props.attachmentId) return `attachment:${props.sessionId}:${props.attachmentId}`;
  if (
    props.sessionId &&
    props.messageId &&
    Number.isInteger(props.artifactIndex) &&
    (props.artifactIndex as number) >= 0
  ) {
    return `artifact:${props.sessionId}:${props.messageId}:${props.artifactIndex}`;
  }
  return '';
}

/** 将多页文档的目标页码合并进预览 URL（一次成型，避免分步赋值触发 iframe 二次加载）。 */
function withPageHash(url: string): string {
  if (!url || !props.page || props.page <= 0) return url;
  return `${url}#page=${props.page}`;
}

// Skill-generated HTML is often a self-contained chart that needs its own
// scripts. Knowledge-base files and chat attachments are untrusted uploads:
// they stay as source, matching the previous text preview.
function allowsHtmlScriptPreview(): boolean {
  return getPreviewSourceKey().startsWith('artifact:');
}

async function fetchPreviewBlob(): Promise<Blob> {
  if (props.sourceBlob) return props.sourceBlob;
  if (props.knowledgeId) {
    return previewKnowledgeFile(props.knowledgeId);
  }
  if (props.sessionId && props.attachmentId) {
    return previewTemporaryAttachment(props.sessionId, props.attachmentId);
  }
  if (
    props.sessionId &&
    props.messageId &&
    Number.isInteger(props.artifactIndex) &&
    (props.artifactIndex as number) >= 0
  ) {
    return downloadArtifact(props.sessionId, props.messageId, props.artifactIndex as number);
  }
  throw new Error('Missing preview source');
}

async function renderMermaid(blob: Blob) {
  const text = await blob.text();
  const svg = await renderMermaidToSvg(text, `file-preview-mermaid-${Date.now()}`);
  if (svg) {
    mermaidSvg.value = sanitizeMarkdownHTML(svg);
    return;
  }
  previewType.value = 'text';
  await renderText(new Blob([text], { type: 'text/plain' }), 'mmd');
}

async function openMermaid() {
  const svg = mermaidSvg.value;
  if (!svg) return;
  // The diagram viewer mounts on body, outside the preview element.
  if (props.active && previewRoot.value?.isConnected) openMermaidFullscreen(svg);
}

async function loadPreview() {
  const sourceKey = getPreviewSourceKey();
  if (!sourceKey) return;
  if (loadedForId === sourceKey) return;
  // 同一 sourceKey 正在加载中：等待已有的加载完成，而不是发起重复请求。
  // 重复请求会拿到不同的签名 URL，导致 iframe src 被设置两次（闪烁）。
  if (loadingForKey === sourceKey && loadingPromise) {
    await loadingPromise;
    return;
  }

  loadingForKey = sourceKey;
  loadingPromise = doLoadPreview(sourceKey);
  try {
    await loadingPromise;
  } finally {
    if (loadingForKey === sourceKey) {
      loadingForKey = '';
      loadingPromise = null;
    }
  }
}

async function doLoadPreview(sourceKey: string) {
  loading.value = true;
  error.value = '';
  htmlViewMode.value = allowsHtmlScriptPreview() ? 'render' : 'source';

  // 记录进入前的直连 PDF 状态：同文件页码切换走 fragment 导航（瞬间完成，无需遮罩），
  // 跨文件/首次加载需要 iframePending 遮罩直到 iframe load，避免白屏闪烁。
  // 注意：必须在 previewType 被新文件类型覆盖之前捕获旧状态。
  const wasDirectPdf = previewType.value === 'pdf' && !!directUrl.value;
  const prevKnowledgeId = props.knowledgeId;

  let ft = resolveFilePreviewExt(props.fileName, props.fileType);
  previewType.value = resolvePreviewKind(ft);

  try {
    // 知识库源文件优先尝试直连预览 URL（COS 预签名 / local /r/ 授权）：
    // PDF/图片/音视频交给浏览器原生加载（自动 Range 分块），几十页大 PDF 首屏秒开、不占 VPS 带宽。
    // 注意顺序：先取到直连 URL、拼好最终形态，再清理旧内容并一次性赋值，
    // 保证 iframe 节点从不被 loading 状态销毁重建（消除二次加载闪烁）。
    let direct = '';
    if (props.knowledgeId) {
      // 优先复用模块级缓存的直连基础 URL：同文件（含跨抽屉打开）走同一预签名 URL，
      // 让浏览器 HTTP 缓存命中，仅变更 #page hash 走同文档 fragment 导航。
      // 若重新请求会拿到全新签名的 URL（query 串不同），浏览器会把 iframe 视为
      // 全新文档而整份重新下载 PDF —— 这就是“加载两次”的根源。
      direct = cachedDirectUrl(props.knowledgeId);
      if (!direct) {
        try {
          const resp = await getKnowledgePreviewUrl(props.knowledgeId);
          direct = resp?.data?.url || '';
        } catch {
          direct = '';
        }
        if (direct) setDirectUrlCache(props.knowledgeId, direct);
      }
    }
    const preliminaryKind = resolvePreviewKind(ft);
    if (
      direct &&
      (preliminaryKind === 'pdf' || preliminaryKind === 'image' || preliminaryKind === 'audio' || preliminaryKind === 'video') &&
      !(props.sourceMode && preliminaryKind === 'pdf')
    ) {
      // URL 与页码合并后单次赋值：Vue 同一次刷新只见最终 src。
      const finalUrl = withPageHash(direct);
      cleanup();
      previewType.value = preliminaryKind;
      previewExt = ft;
      directUrl.value = finalUrl;
      loadedForId = sourceKey;
      const sameFilePageJump = preliminaryKind === 'pdf' && wasDirectPdf && prevKnowledgeId === props.knowledgeId;
      iframePending.value = preliminaryKind === 'pdf' && !sameFilePageJump;
      loading.value = false;
      await nextTick();
      scheduleLocate();
      return;
    }

    // 直连不可用：回退 Blob 整包下载（此时才清理旧内容）。
    cleanup();
    const rawBlob = await fetchPreviewBlob();
    let kind = resolvePreviewKind(ft);
    if (kind === 'unsupported') {
      const sample = new Uint8Array(await rawBlob.slice(0, FILE_PREVIEW_SNIFF_BYTES).arrayBuffer());
      const sniffed = sniffPreview(sample);
      kind = sniffed.kind;
      if (sniffed.ext) ft = sniffed.ext;
    }
    previewType.value = kind;

    if (kind === 'unsupported') {
      loading.value = false;
      return;
    }

    const blob = ensureBlobType(rawBlob, ft);
    loadedForId = sourceKey;

    loading.value = false;
    await nextTick();

    previewExt = ft;
    switch (kind) {
      case 'pdf': {
        if (props.sourceMode) {
          pdfData.value = await blob.arrayBuffer();
        } else {
          const baseUrl = URL.createObjectURL(blob);
          blobUrl.value = withPageHash(baseUrl);
        }
        break;
      }
      case 'image':
      case 'audio':
      case 'video': {
        blobUrl.value = URL.createObjectURL(blob);
        break;
      }
      case 'epub': {
        const rendered = await renderEpubPreview(await blob.arrayBuffer());
        epubObjectUrls = rendered.objectUrls;
        epubHtml.value = sanitizeHTML(rendered.html);
        break;
      }
      case 'html': {
        if (allowsHtmlScriptPreview()) {
          const previewHtml = buildHtmlPreview(await blob.text());
          blobUrl.value = URL.createObjectURL(new Blob([previewHtml], { type: 'text/html;charset=utf-8' }));
        }
        await renderText(blob, ft || 'html');
        break;
      }
      case 'docx': {
        await renderDocx(blob);
        break;
      }
      case 'excel': {
        await renderExcel(blob, ft);
        break;
      }
      case 'text': {
        await renderText(blob, ft);
        break;
      }
      case 'markdown': {
        await renderMarkdown(blob);
        break;
      }
      case 'pptx': {
        const prepared = await preparePptxPreview(await blob.arrayBuffer());
        if (getPreviewSourceKey() !== sourceKey || !props.active) return;
        pptxSlideCount = prepared.slideCount;
        pptxData.value = prepared.data;
        break;
      }
      case 'mermaid': {
        await renderMermaid(blob);
        break;
      }
    }
    // PPTX renders asynchronously and locates from its rendered event.
    if (kind !== 'pptx') scheduleLocate();
  } catch (err: any) {
    console.error('Document preview failed:', err);
    error.value = err?.message || t('preview.loadFailed');
  } finally {
    loading.value = false;
  }
}

function cleanup() {
  directUrl.value = '';
  iframePending.value = false;
  if (blobUrl.value) {
    // blobUrl 可能带 #page=N 后缀（多页定位），revoke 时按基础 URL 清理。
    URL.revokeObjectURL(blobUrl.value.split('#')[0]);
    blobUrl.value = '';
  }
  textContent.value = '';
  highlightedCode.value = '';
  markdownHtml.value = '';
  excelHtml.value = '';
  mermaidSvg.value = '';
  htmlViewMode.value = 'render';
  pptxData.value = null;
  pdfData.value = null;
  for (const url of epubObjectUrls) URL.revokeObjectURL(url);
  epubObjectUrls = [];
  epubHtml.value = '';
  excelSheetNames = [];
  clearLocateMarks();
  pptxSlideCount = 0;
  imageNaturalWidth.value = 0;
  imageNaturalHeight.value = 0;
  loadedForId = '';
  if (docxContainer.value) {
    docxContainer.value.innerHTML = '';
  }
}

// ── Citation locating ──
// Each preview kind reveals a SourceLocateRequest its own way: structural
// locators first (page, block, slide, rows, section, time), then the quoted
// text inside whatever they narrowed down, then the quotes anywhere.

type ImageMark = { left: number; top: number; width: number; height: number };
const imageMarks = ref<ImageMark[]>([]);
const audioQuote = ref('');
const audioRange = ref('');
let clearLocate: Array<() => void> = [];

function clearLocateMarks() {
  for (const clear of clearLocate) clear();
  clearLocate = [];
  imageMarks.value = [];
  audioQuote.value = '';
  audioRange.value = '';
}

let locateScheduled = false;
function scheduleLocate() {
  if (!props.locate || locateScheduled) return;
  locateScheduled = true;
  void nextTick(() => {
    locateScheduled = false;
    applyLocate();
  });
}

watch(
  () => props.locate?.token,
  () => {
    if (loadedForId && !loading.value) scheduleLocate();
  },
);

const NOT_FOUND: SourceLocateResult = { found: false, precise: false };

function findQuoteRange(root: Node | null | undefined, quotes: string[]): Range | null {
  if (!root) return null;
  const index = buildTextIndex(root);
  for (const quote of quotes) {
    const range = findTextRange(root, quote, index);
    if (range) return range;
  }
  return null;
}

function reveal(container: Element | null | undefined, target: Range | Element | null | undefined) {
  if (!container || !target) return;
  scrollRectIntoContainer(container, target.getBoundingClientRect());
}

function commitMarks(ranges: Range[], elements: Element[]) {
  if (ranges.length) clearLocate.push(highlightRanges(ranges));
  if (elements.length) clearLocate.push(highlightElements(elements));
}

/** Reveal the first quote found under `root`, scrolling `container`. */
function locateQuote(root: Element | null | undefined, container: Element | null | undefined, quotes: string[]): SourceLocateResult {
  const range = findQuoteRange(root, quotes);
  if (!range) return NOT_FOUND;
  commitMarks([range], []);
  reveal(container, range);
  return { found: true, precise: true };
}

/** Only exact evidence may narrow a structural source region. */
function evidenceQuotes(request: SourceLocateRequest): string[] {
  return request.scope ? [request.scope] : request.quotes;
}

function locateDocx(request: SourceLocateRequest): SourceLocateResult {
  const root = docxContainer.value;
  if (!root) return NOT_FOUND;
  const blocks = Array.from(root.querySelectorAll('section > article')).flatMap((article) =>
    Array.from(article.children).filter((c) => c.tagName === 'P' || c.tagName === 'TABLE'),
  );
  if (request.imageDigest) {
    const images = [...root.querySelectorAll('img')].filter(image => image.dataset.sourceImageDigest === request.imageDigest);
    if (images.length !== 1) return { ...NOT_FOUND, ...(images.length > 1 ? { reason: 'ambiguous' } : {}) };
    commitMarks([], images); reveal(root, images[0]);
    return { found: true, precise: false, granularity: 'block' };
  }
  if (request.imageContext) {
    const { before: beforeQuote, after: afterQuote } = request.imageContext;
    const before = beforeQuote ? findQuoteRange(root, [beforeQuote]) : null;
    const after = afterQuote ? findQuoteRange(root, [afterQuote]) : null;
    if ((beforeQuote && !before) || (afterQuote && !after) || (!before && !after)) return NOT_FOUND;
    const from = before?.cloneRange() || document.createRange();
    const to = after?.cloneRange() || document.createRange();
    if (before) from.collapse(false);
    else { from.selectNodeContents(root); from.collapse(true); }
    if (after) to.collapse(true);
    else { to.selectNodeContents(root); to.collapse(false); }
    // A chunk can end immediately after its image. In that case bound the
    // search by the next nonempty original paragraph, not the document end.
    for (const block of blocks) {
      if (!block.textContent?.trim()) continue;
      const range = document.createRange(); range.selectNodeContents(block);
      if (!after && range.compareBoundaryPoints(Range.START_TO_START, from) >= 0) { to.setStartBefore(block); to.collapse(true); break; }
      if (!before && range.compareBoundaryPoints(Range.END_TO_END, to) <= 0) { from.setStartAfter(block); from.collapse(true); }
    }
    const images = [...root.querySelectorAll('img')].filter(image => {
      const range = document.createRange(); range.selectNode(image);
      return range.compareBoundaryPoints(Range.START_TO_START, from) >= 0 &&
        range.compareBoundaryPoints(Range.END_TO_END, to) <= 0;
    });
    if (images.length !== 1) return NOT_FOUND;
    commitMarks([], images); reveal(root, images[0]);
    return { found: true, precise: false, granularity: 'block' };
  }
  const excerpt = request.quotes.length === 1 ? request.quotes[0] : '';
  if (excerpt && request.scope && excerpt !== request.scope && findInText(request.scope, excerpt)) {
    const range = findQuoteRange(root, [excerpt]);
    if (range) { commitMarks([range], []); reveal(root, range); return { found: true, precise: true }; }
  }
  const ranges: Range[] = [];
  let missed = false, partial = false, narrowed = false;
  for (const loc of request.locators.filter((l) => l.type === 'docx' && l.mapping === 'exact' && l.block)) {
    const el = blocks[(loc.block as number) - 1];
    const quote = sourceQuoteText(loc.quote || '');
    const sentence = request.sentence && findInText(quote, request.sentence) ? request.sentence : '';
    // Pagination may split Word paragraphs. Revalidate the complete quote
    // globally before accepting a shifted block, and reject duplicate matches.
    const region = quote ? findQuoteRange(el, [quote]) || findQuoteRange(root, [quote]) : null;
    if (!region) { missed = true; continue; }
    const range = sentence ? findQuoteRange(region.commonAncestorContainer.nodeType === Node.TEXT_NODE ? region.commonAncestorContainer.parentElement : region.commonAncestorContainer, [sentence]) : region;
    ranges.push(range || region);
    narrowed ||= !!sentence && !!range;
    partial ||= !!loc.partial;
  }
  if (!ranges.length) return locateQuote(root, root, evidenceQuotes(request));
  const complete = !missed && (!partial || narrowed || !!findQuoteRange(root, evidenceQuotes(request)));
  commitMarks(ranges, []);
  reveal(root, ranges[0]);
  return { found: true, precise: complete, ...(!complete ? { reason: 'partial' } : {}) };
}

function locatePptx(request: SourceLocateRequest): SourceLocateResult {
  const box = previewContent.value;
  if (!box) return NOT_FOUND;
  const slides = Array.from(box.querySelectorAll('.pptx-preview-slide-wrapper'));
  const locators = request.locators.filter((l) => l.type === 'slide' && l.mapping === 'exact' && !l.partial && l.slide);
  if (!locators.length) return locateQuote(box, box, evidenceQuotes(request));
  const ranges: Range[] = [], regions: Element[] = [];
  for (const loc of locators) {
    const slide = slides[(loc.slide as number) - 1];
    if (!slide) return NOT_FOUND;
    const own = slide.querySelector('.slide-wrapper') || slide;
    const range = loc.quote ? findQuoteRange(own, [loc.quote]) : null;
    if (range) ranges.push(range); else regions.push(slide);
  }
  commitMarks(ranges, regions);
  reveal(box, ranges[0] || regions[0]);
  return { found: true, precise: !regions.length, granularity: regions.length ? 'block' : 'text' };
}

/** Map sheet row numbers to table rows using the cell ids SheetJS emits. */
function sheetRows(table: Element): Map<number, Element> {
  const rows = new Map<number, Element>();
  for (const cell of Array.from(table.querySelectorAll('td[id]'))) {
    const match = /-[A-Z]+(\d+)$/.exec(cell.id);
    const tr = cell.closest('tr');
    if (match && tr && !rows.has(Number(match[1]))) rows.set(Number(match[1]), tr);
  }
  return rows;
}

function locateExcel(request: SourceLocateRequest): SourceLocateResult {
  const box = previewContent.value;
  if (!box) return NOT_FOUND;
  const locators = request.locators.filter((l) => l.type === 'sheet' && l.mapping === 'exact' && !l.partial && l.row_start);
  const targets: Element[] = [];
  for (const loc of locators) {
    const byName = loc.sheet ? excelSheetNames.indexOf(loc.sheet) : -1;
    if (byName < 0) continue;
    const sheetIdx = byName;
    const table = box.querySelector(`#user-content-sheet-${sheetIdx}`) || box.querySelector(`#sheet-${sheetIdx}`);
    if (!table) continue;
    const rows = sheetRows(table);
    const last = Math.min(loc.row_end || loc.row_start || 0, (loc.row_start || 0) + 200);
    for (let r = loc.row_start as number; r <= last; r++) {
      const tr = rows.get(r);
      if (tr && !targets.includes(tr)) targets.push(tr);
    }
  }
  if (targets.length) {
    commitMarks([], targets);
    reveal(box, targets[0]);
    return { found: true, precise: false, granularity: 'block' };
  }
  return locateQuote(box, box, evidenceQuotes(request));
}

function locateEpub(request: SourceLocateRequest): SourceLocateResult {
  const box = previewContent.value;
  if (!box) return NOT_FOUND;
  const locators = request.locators.filter((l) => l.type === 'section' && l.mapping === 'exact' && !l.partial && l.section);
  if (!locators.length) return locateQuote(box, box, evidenceQuotes(request));
  const ranges: Range[] = [], regions: Element[] = [];
  for (const loc of locators) {
    const section = box.querySelector(`[data-epub-section="${loc.section}"]`);
    if (!section) return NOT_FOUND;
    const range = loc.quote ? findQuoteRange(section, [loc.quote]) : null;
    if (range) ranges.push(range); else regions.push(section);
  }
  commitMarks(ranges, regions);
  reveal(box, ranges[0] || regions[0]);
  return { found: true, precise: !regions.length, granularity: regions.length ? 'block' : 'text' };
}

function locateText(request: SourceLocateRequest): SourceLocateResult {
  const box = previewContent.value;
  const code = box?.querySelector('code') || box;
  if (!box || !code) return NOT_FOUND;
  // Validate stored offsets against their full quote. A manual edit or a
  // pretty-printed JSON document must not reuse offsets from another version.
  const locators = request.locators.filter((l) => l.type === 'text' && !l.partial && (l.end || 0) > (l.start || 0));
  if (locators.length && previewType.value === 'text' && !shouldPrettyPrintJson(previewExt)) {
    const ranges = locators.map((l) => rangeForOffsets(code, l.start || 0, l.end || 0));
    if (ranges.every((r, i) => r && locators[i].quote && findInText(r.toString(), locators[i].quote!))) {
      commitMarks(ranges as Range[], []);
      reveal(box, ranges[0]);
      return { found: true, precise: true };
    }
  }
  return locateQuote(code, box, evidenceQuotes(request));
}

function locateMarkdown(request: SourceLocateRequest): SourceLocateResult {
  const box = previewContent.value;
  if (!box) return NOT_FOUND;
  const markdown = request.sourceMarkdown ?? request.scope ?? request.quotes[0] ?? '';
  const match = findMarkdownSourceRange(box, markdown, request.sentence);
  if (!match) return NOT_FOUND;
  commitMarks(match.ranges || [match.range], []);
  reveal(box, match.range);
  return { found: true, precise: match.exact, ...(!match.exact ? { granularity: 'block' } : {}) };
}

function locateImage(request: SourceLocateRequest): SourceLocateResult {
  const boxes = request.locators
    .filter((l) => l.type === 'pdf' && l.mapping === 'exact' && !l.partial && (l.page || 1) === 1 && l.bbox?.length === 4)
    .map((l) => {
      const [x0, y0, x1, y1] = l.bbox as number[];
      return { left: x0, top: y0, width: x1 - x0, height: y1 - y0 };
    });
  imageMarks.value = boxes;
  // Without regions the whole image is the cited source.
  return { found: true, precise: false, granularity: 'block' };
}

function formatClock(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

function locateAudio(request: SourceLocateRequest): SourceLocateResult {
  const audio = previewContent.value as HTMLAudioElement | null;
  const loc = request.locators.find((l) => l.type === 'time' && (l.end_ms || 0) > (l.start_ms || 0));
  if (!audio || !loc) return NOT_FOUND;
  // Seek only; playback stays the reader's choice.
  const seek = () => {
    audio.currentTime = (loc.start_ms || 0) / 1000;
  };
  if (audio.readyState >= 1) {
    seek();
  } else {
    // A newer locate clears this, so an older citation cannot win the seek.
    audio.addEventListener('loadedmetadata', seek, { once: true });
    clearLocate.push(() => audio.removeEventListener('loadedmetadata', seek));
  }
  audioRange.value = `${formatClock(loc.start_ms || 0)} – ${formatClock(loc.end_ms || 0)}`;
  audioQuote.value = loc.quote || '';
  return { found: true, precise: true };
}

function applyLocate() {
  const request = props.locate;
  clearLocateMarks();
  // The pdf.js viewer locates on its own and reports through @located.
  if (!request || previewType.value === 'pdf') return;
  if (request.unavailable) { emit('located', { ...NOT_FOUND, reason: 'unavailable' }); return; }
  let result = NOT_FOUND;
  try {
    switch (previewType.value) {
      case 'docx': result = locateDocx(request); break;
      case 'pptx': result = locatePptx(request); break;
      case 'excel': result = locateExcel(request); break;
      case 'epub': result = locateEpub(request); break;
      case 'text':
      case 'html': result = locateText(request); break;
      case 'markdown': result = locateMarkdown(request); break;
      case 'image': result = locateImage(request); break;
      case 'audio': result = locateAudio(request); break;
    }
  } catch (err) {
    console.warn('Locating the citation failed:', err);
  }
  emit('located', result);
}

watch(
  () => [props.active, props.knowledgeId, props.sessionId, props.attachmentId, props.messageId, props.artifactIndex, props.sourceBlob, props.fileName, props.fileType, props.page],
  ([active]) => {
    if (active && getPreviewSourceKey()) {
      loadPreview();
    }
  },
  { immediate: true }
);

onUnmounted(() => {
  cleanup();
});
</script>

<template>
  <div ref="previewRoot" tabindex="-1" :aria-label="fileName" class="document-preview" :class="{ 'fill-height': fillHeight }">
    <!-- Loading：改为底部覆盖层（v-show 语义），不再与内容 v-if 互斥，
         避免 iframe 因 loading 状态反复销毁重建引发二次加载闪烁。 -->
    <div v-if="loading || iframePending" class="preview-loading preview-loading--overlay">
      <t-loading size="medium" />
      <span class="loading-text">{{ $t('preview.loading') }}</span>
    </div>

    <!-- Error -->
    <div v-if="error" class="preview-error">
      <t-icon name="error-circle" size="48px" />
      <p>{{ error }}</p>
      <t-button theme="primary" size="small" @click="loadedForId = ''; loadPreview()">
        {{ $t('preview.retry') }}
      </t-button>
    </div>

    <!-- Unsupported -->
    <div v-else-if="previewType === 'unsupported'" class="preview-unsupported">
      <t-icon name="file-unknown" size="48px" />
      <p>{{ $t('preview.unsupported') }}</p>
      <p class="unsupported-hint">{{ $t('preview.unsupportedHint') }}</p>
    </div>

    <!-- PDF -->
    <div v-else-if="previewType === 'pdf' && pdfData" class="preview-pdf preview-pdf--source">
      <PdfSourceViewer :data="pdfData" :file-name="fileName" :locate="locate" @located="(r) => emit('located', r)" />
    </div>
    <div v-else-if="previewType === 'pdf' && (blobUrl || directUrl)" class="preview-pdf">
      <iframe ref="previewContent" tabindex="0" :title="fileName" :src="blobUrl || directUrl" class="pdf-iframe" @load="onPreviewFrameLoad" />
    </div>

    <!-- HTML: artifacts render in a unique-origin iframe; other sources stay as source. -->
    <div v-else-if="previewType === 'html'" class="preview-html">
      <iframe
        v-if="allowsHtmlScriptPreview() && htmlViewMode === 'render' && blobUrl"
        ref="previewContent"
        tabindex="0"
        :title="fileName"
        :src="blobUrl"
        class="html-iframe"
        sandbox="allow-scripts"
        referrerpolicy="no-referrer"
        @load="onPreviewFrameLoad"
      />
      <pre v-else-if="!allowsHtmlScriptPreview() || htmlViewMode === 'source'" ref="previewContent" tabindex="0" :aria-label="fileName" class="code-preview"><code class="hljs" v-html="highlightedCode"></code></pre>
    </div>

    <!-- Image -->
    <div v-else-if="previewType === 'image' && (blobUrl || directUrl)" ref="previewContent" tabindex="0" :aria-label="fileName" class="preview-image">
      <div class="image-wrapper">
        <div class="image-frame">
          <img :src="blobUrl || directUrl" :alt="fileName" @load="onImageLoad" />
          <div
            v-for="(mark, i) in imageMarks"
            :key="i"
            class="image-locate-mark"
            aria-hidden="true"
            :style="{ left: `${mark.left * 100}%`, top: `${mark.top * 100}%`, width: `${mark.width * 100}%`, height: `${mark.height * 100}%` }"
          />
        </div>
        <div v-if="imageNaturalWidth" class="image-info">
          {{ imageNaturalWidth }} × {{ imageNaturalHeight }} px
        </div>
      </div>
    </div>

    <!-- DOCX -->
    <div v-else-if="previewType === 'docx'" class="preview-docx">
      <div ref="docxContainer" tabindex="0" :aria-label="fileName" class="docx-container" />
    </div>

    <!-- PPTX -->
    <div v-else-if="previewType === 'pptx' && pptxData" ref="previewContent" tabindex="0" :aria-label="fileName" class="preview-pptx">
      <vue-office-pptx :key="loadedForId" :src="pptxData" @rendered="onPptxRendered" @error="(e: any) => { error = e?.message || $t('preview.loadFailed'); }" />
    </div>

    <!-- Excel -->
    <div v-else-if="previewType === 'excel' && excelHtml" class="preview-excel">
      <div ref="previewContent" tabindex="0" :aria-label="fileName" class="excel-container" v-html="excelHtml" />
    </div>

    <!-- EPUB -->
    <div v-else-if="previewType === 'epub' && epubHtml" ref="previewContent" tabindex="0" :aria-label="fileName" class="preview-markdown preview-epub">
      <div class="markdown-body" v-html="epubHtml" />
    </div>

    <!-- Markdown -->
    <div v-else-if="previewType === 'markdown' && markdownHtml" ref="previewContent" tabindex="0" :aria-label="fileName" class="preview-markdown">
      <div class="markdown-body" v-html="markdownHtml" />
    </div>

    <!-- Text / Code -->
    <div v-else-if="previewType === 'text' && highlightedCode" class="preview-text">
      <pre ref="previewContent" tabindex="0" :aria-label="fileName" class="code-preview"><code class="hljs" v-html="highlightedCode"></code></pre>
    </div>

    <!-- Mermaid -->
    <div v-else-if="previewType === 'mermaid' && mermaidSvg" ref="previewContent" tabindex="0" :aria-label="fileName" class="preview-mermaid" @click="openMermaid">
      <div class="mermaid-body" v-html="mermaidSvg" />
    </div>

    <!-- Audio -->
    <div v-else-if="previewType === 'audio' && (blobUrl || directUrl)" class="preview-audio">
      <div class="audio-wrapper">
        <t-icon name="sound" size="48px" />
        <p class="audio-filename">{{ fileName }}</p>
        <audio ref="previewContent" controls :src="blobUrl || directUrl" class="audio-element">
          {{ $t('preview.audioNotSupported') }}
        </audio>
        <div v-if="audioRange" class="audio-locate">
          <span class="audio-locate__time">{{ audioRange }}</span>
          <p v-if="audioQuote" class="audio-locate__quote">{{ audioQuote }}</p>
        </div>
      </div>
    </div>

    <!-- Video -->
    <div v-else-if="previewType === 'video' && (blobUrl || directUrl)" class="preview-video">
      <video ref="previewContent" controls playsinline :src="blobUrl || directUrl" class="video-element">
        {{ $t('preview.videoNotSupported') }}
      </video>
    </div>
  </div></template>

<style scoped lang="less">
// ── Design tokens ──
@border-color: var(--td-component-stroke);
@border-radius: var(--app-radius-sm);
@bg-white: var(--td-bg-color-container);
@bg-subtle: var(--td-bg-color-container);
@bg-muted: var(--td-bg-color-secondarycontainer);
@text-primary: var(--td-text-color-primary);
@text-secondary: var(--td-text-color-secondary);
@text-tertiary: var(--td-text-color-placeholder);
@text-disabled: var(--td-text-color-disabled);
@accent: var(--td-brand-color);
@accent-hover: var(--td-brand-color-active);
@accent-bg: var(--td-success-color-light);
@accent-bg-hover: var(--td-success-color-light);
@error-color: var(--td-error-color);
@table-border: var(--td-component-stroke);
@preview-max-h: calc(100vh - 200px);
// Note: <html> carries a `zoom` multiplier for font-size control, so 100vh
// is evaluated against the unscaled viewport and the resulting max-height
// may exceed the real viewport by the zoom factor (≤12.5% at "large").
// That produces an extra bit of scroll inside the non-fullscreen preview,
// which is acceptable for document reading. Not worth the complexity of
// inverse-scaling here.
@transition: all var(--app-motion-base) ease;

// ── Shared container mixin ──
.preview-container() {
  border: 1px solid @border-color;
  border-radius: @border-radius;
  overflow: auto;
  max-height: @preview-max-h;
  background: @bg-white;
}

.document-preview {
  min-height: 200px;
  position: relative;

  &.fill-height {
    height: 100%;
    min-height: 0;
    display: flex;
    flex-direction: column;

    .preview-pdf,
    .preview-pptx,
    .preview-docx,
    .preview-image,
    .preview-excel,
    .preview-markdown,
    .preview-text,
    .preview-html,
    .preview-mermaid,
    .preview-audio,
    .preview-video,
    .preview-loading,
    .preview-error,
    .preview-unsupported {
      flex: 1;
      min-height: 0;
      max-height: none;
    }

    .preview-pdf {
      height: auto;
      min-height: 0;
    }

    .preview-pptx {
      overflow: auto;
    }

    .preview-docx,
    .preview-excel,
    .preview-text {
      display: flex;
      flex-direction: column;

      .docx-container,
      .excel-container,
      .code-preview {
        flex: 1;
        min-height: 0;
        max-height: none;
        height: auto;
      }
    }

    .preview-image .image-wrapper img {
      max-height: 100%;
    }

    .preview-excel .excel-container,
    .preview-markdown,
    .preview-text .code-preview,
    .preview-html .code-preview {
      max-height: none;
    }

    .preview-html .html-iframe {
      height: 100%;
    }

    .preview-html {
      height: auto;
      min-height: 0;
    }
  }
}

// ── States ──
.preview-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
  .loading-text { color: @text-tertiary; font-size: var(--app-text-base); }
}

// 覆盖层形态：盖在内容上方，而非与内容 v-if 互斥销毁（保护 iframe 不被重建）。
.preview-loading--overlay {
  position: absolute;
  inset: 0;
  z-index: 2;
  padding: 24px 20px;
  border-radius: @border-radius;
  background: color-mix(in srgb, var(--td-bg-color-container) 82%, transparent);
  backdrop-filter: blur(1px);
}

.preview-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 12px;
  color: @error-color;
  p { margin: 0; font-size: var(--app-text-base); color: @text-secondary; }
}

.preview-unsupported {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 12px;
  color: @text-disabled;
  p { margin: 0; font-size: var(--app-text-base); color: @text-secondary; }
  .unsupported-hint { font-size: var(--app-text-sm); color: @text-tertiary; }
}

// ── PDF ──
.preview-pdf {
  width: 100%;
  height: @preview-max-h;
  min-height: 500px;
  .pdf-iframe {
    width: 100%;
    height: 100%;
    border: none;
    border-radius: @border-radius;
  }
}

// ── HTML ──
.preview-html {
  width: 100%;
  height: @preview-max-h;
  min-height: 420px;
  display: flex;
  flex-direction: column;
  .html-iframe {
    flex: 1;
    width: 100%;
    min-height: 0;
    border: 1px solid @border-color;
    border-radius: @border-radius;
    background: @bg-white;
  }
  .code-preview {
    .preview-container();
    flex: 1;
    min-height: 0;
    margin: 0;
    padding: 16px;
    background: @bg-subtle;
    font-size: var(--app-text-md);
    line-height: 1.6;
    code {
      white-space: pre;
      word-wrap: normal;
      display: block;
      background: transparent;
    }
  }
}

// ── Mermaid ──
.preview-mermaid {
  .preview-container();
  display: flex;
  justify-content: center;
  padding: 24px 16px;
  cursor: zoom-in;
  .mermaid-body {
    max-width: 100%;
    :deep(svg) {
      max-width: 100%;
      height: auto;
    }
  }
}

// ── PDF (pdf.js, source mode) ──
.preview-pdf--source {
  height: @preview-max-h;
  border: 1px solid @border-color;
  border-radius: @border-radius;
  overflow: hidden;
}

// ── Image ──
.image-frame {
  position: relative;
  display: inline-block;
  line-height: 0;
}

.image-locate-mark {
  position: absolute;
  pointer-events: none;
  border-radius: var(--app-radius-xs);
  background: var(--app-source-highlight-soft-bg);
  outline: 1px solid var(--app-source-highlight-border);
}

.preview-image {
  overflow: auto;
  display: flex;
  justify-content: center;
  padding: 20px 0;
  .image-wrapper {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    img {
      max-width: 100%;
      max-height: calc(100vh - 280px);
      border-radius: @border-radius;
      box-shadow: 0 2px 12px color-mix(in srgb, var(--td-brand-color) 8%, transparent);
      object-fit: contain;
    }
    .image-info { font-size: var(--app-text-sm); color: @text-tertiary; }
  }
}

// ── Markdown ──
.preview-markdown {
  .preview-container();
  padding: 20px 24px;
}

// ── DOCX ──
.preview-docx {
  .docx-container { .preview-container(); }
}

// ── PPTX ──
.preview-pptx {
  max-height: @preview-max-h;
  min-height: 500px;
  border: 1px solid @border-color;
  border-radius: @border-radius;
  overflow: auto;
  background: @bg-subtle;

  :deep(.pptx-preview-wrapper) {
    height: auto !important;
    overflow-y: visible !important;
  }
}

// ── Excel ──
.preview-excel {
  .excel-container { .preview-container(); }
}

// ── Text / Code ──
.preview-text {
  .code-preview {
    .preview-container();
    margin: 0;
    padding: 16px;
    background: @bg-subtle;
    font-size: var(--app-text-md);
    line-height: 1.6;
    code {
      white-space: pre;
      word-wrap: normal;
      display: block;
      background: transparent;
    }
  }
}

// ── Audio ──
.preview-audio {
  display: flex;
  justify-content: center;
  padding: 40px 20px;
  .audio-wrapper {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    color: @text-secondary;
    .audio-filename { font-size: var(--app-text-base); color: @text-primary; margin: 0; }
    .audio-element { width: 100%; max-width: 480px; }
  }
  .audio-locate {
    width: 100%;
    max-width: 480px;
    padding: 10px 12px;
    border-radius: @border-radius;
    background: var(--app-source-highlight-soft-bg);
    color: @text-primary;
    &__time {
      font-size: var(--app-text-sm);
      font-variant-numeric: tabular-nums;
      color: @text-secondary;
    }
    &__quote {
      margin: 4px 0 0;
      font-size: var(--app-text-md);
      line-height: 1.6;
    }
  }
}

// ── Video ──
.preview-video {
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 16px;
  min-height: 280px;
  .video-element {
    width: 100%;
    max-height: calc(100vh - 240px);
    border-radius: @border-radius;
    background: #000;
  }
}

// ── Deep styles (v-html / third-party components) ──

// Shared table mixin for v-html content
.preview-table(@header-bg: @accent-bg; @hover-bg: @accent-bg) {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--app-text-md);
  th, td {
    border: 1px solid @table-border;
    padding: 6px 12px;
    text-align: left;
  }
  th {
    background: @header-bg;
    font-weight: 600;
    color: @text-primary;
  }
  tr:hover td {
    background: @hover-bg;
    transition: @transition;
  }
}

:deep(.markdown-body) {
  font-size: var(--app-text-base);
  line-height: 1.7;
  color: @text-primary;
  word-break: break-word;

  h1, h2, h3, h4, h5, h6 {
    margin-top: 20px;
    margin-bottom: 10px;
    font-weight: 600;
    line-height: 1.4;
  }
  h1 { font-size: var(--app-text-4xl); border-bottom: 1px solid @border-color; padding-bottom: 8px; }
  h2 { font-size: var(--app-text-3xl); border-bottom: 1px solid @border-color; padding-bottom: 6px; }
  h3 { font-size: 17px; }

  p { margin: 8px 0; }
  blockquote {
    margin: 12px 0;
    padding: 8px 16px;
    border-left: 4px solid @border-color;
    background: @bg-subtle;
    color: var(--td-text-color-secondary);
  }
  ul, ol { padding-left: 24px; margin: 8px 0; }
  li { margin: 4px 0; }

  table {
    .preview-table(@bg-muted; var(--td-bg-color-container-hover));
    margin: 12px 0;
  }

  pre {
    margin: 12px 0;
    padding: 14px;
    background: @bg-subtle;
    border-radius: @border-radius;
    overflow: auto;
    font-size: var(--app-text-md);
    line-height: 1.5;
    code { background: transparent; padding: 0; }
  }
  code {
    background: var(--td-bg-color-secondarycontainer);
    padding: 2px 6px;
    border-radius: 3px;
    font-size: 0.9em;
  }
  img { max-width: 100%; border-radius: var(--app-radius-xs); }
  hr { border: none; border-top: 1px solid @border-color; margin: 20px 0; }
  a { color: @accent; text-decoration: none; &:hover { color: @accent-hover; text-decoration: underline; } }
  strong { font-weight: 600; }
}

:deep(.docx-preview-wrapper) {
  padding: 20px;
  max-width: 100%;
  width: 100%;
  box-sizing: border-box;
  overflow-x: auto; // 如果内容过宽，允许水平滚动而不是溢出
  
  // 约束所有子元素的宽度
  * {
    max-width: 100%;
    box-sizing: border-box;
  }
  
  // 特别处理表格
  table {
    width: 100%;
    table-layout: auto;
    word-wrap: break-word;
  }
  
  // 处理图片
  img {
    max-width: 100%;
    height: auto;
  }
  
  // 处理可能的固定宽度元素
  [style*="width"] {
    max-width: 100% !important;
  }
}

:deep(.vue-office-pptx) {
  width: 100%;
  min-height: 100%;
}

:deep(.vue-office-pptx-main) {
  width: 100%;
  min-height: 100%;
}

:deep(.excel-sheet) {
  padding: 0;
  .excel-sheet-name {
    position: sticky;
    top: 0;
    background: @accent-bg;
    padding: 8px 16px;
    font-weight: 600;
    font-size: var(--app-text-md);
    color: @text-primary;
    border-bottom: 1px solid @border-color;
    z-index: 1;
  }
  table {
    .preview-table();
    th, td {
      white-space: nowrap;
      max-width: 300px;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }
}
</style>

<!-- highlight.js github.css is a light theme imported globally; its token
     colors (notably the base #24292e) become unreadable on the dark
     container background used in dark mode. Remap the palette to the
     github-dark colors when dark mode is active. Non-scoped on purpose so
     it covers every hljs block (txt/code preview, markdown code fences). -->
<style lang="less">
html[theme-mode="dark"] {
  .hljs {
    color: #c9d1d9;
    background: transparent;
  }
  .hljs-doctag,
  .hljs-keyword,
  .hljs-meta .hljs-keyword,
  .hljs-template-tag,
  .hljs-template-variable,
  .hljs-type,
  .hljs-variable.language_ {
    color: #ff7b72;
  }
  .hljs-title,
  .hljs-title.class_,
  .hljs-title.class_.inherited__,
  .hljs-title.function_ {
    color: #d2a8ff;
  }
  .hljs-attr,
  .hljs-attribute,
  .hljs-literal,
  .hljs-meta,
  .hljs-number,
  .hljs-operator,
  .hljs-variable,
  .hljs-selector-attr,
  .hljs-selector-class,
  .hljs-selector-id {
    color: #79c0ff;
  }
  .hljs-regexp,
  .hljs-string,
  .hljs-meta .hljs-string {
    color: #a5d6ff;
  }
  .hljs-built_in,
  .hljs-symbol {
    color: #ffa657;
  }
  .hljs-comment,
  .hljs-code,
  .hljs-formula {
    color: #8b949e;
  }
  .hljs-name,
  .hljs-quote,
  .hljs-selector-tag,
  .hljs-selector-pseudo {
    color: #7ee787;
  }
  .hljs-subst {
    color: #c9d1d9;
  }
  .hljs-section {
    color: #1f6feb;
    font-weight: bold;
  }
  .hljs-bullet {
    color: #f2cc60;
  }
  .hljs-emphasis {
    color: #c9d1d9;
    font-style: italic;
  }
  .hljs-strong {
    color: #c9d1d9;
    font-weight: bold;
  }
  .hljs-addition {
    color: #aff5b4;
    background-color: #033a16;
  }
  .hljs-deletion {
    color: #ffdcd7;
    background-color: #67060c;
  }
}
</style>

<style lang="less">
/* Citation highlights live outside the scoped block: they apply to DOM that
   third-party renderers (docx-preview, pptx-preview, SheetJS) create. */
::highlight(source-locate) {
  background-color: var(--app-source-highlight-bg);
}

mark.source-locate-mark {
  background-color: var(--app-source-highlight-bg);
  color: inherit;
  border-radius: var(--app-radius-xs);
  box-decoration-break: clone;
  -webkit-box-decoration-break: clone;
}

.source-locate-block {
  background-color: var(--app-source-highlight-soft-bg) !important;
  box-shadow: inset 3px 0 var(--app-source-highlight-border);
}

/* Cell backgrounds can obscure a row's marker. Paint each cell once and
   keep the accent on the leading edge, without boxing every table row. */
tr.source-locate-block {
  background-color: transparent !important;
  box-shadow: none;
}

tr.source-locate-block > td {
  background-color: var(--app-source-highlight-soft-bg) !important;
}

tr.source-locate-block > td:first-child {
  box-shadow: inset 3px 0 var(--app-source-highlight-border);
}

img.source-locate-block {
  outline: 1px solid var(--app-source-highlight-border);
  outline-offset: 2px;
}
</style>
