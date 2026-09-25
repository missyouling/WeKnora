<template>
  <div class="fleet-cat-panel">
    <div class="panel-head">
      <span class="panel-title">分类大项</span>
      <t-button size="small" theme="primary" variant="outline" @click="startAdd">
        <template #icon><t-icon name="add" size="14px" /></template>新增分类
      </t-button>
    </div>
    <div v-if="!categories.length" class="panel-empty">
      暂无分类，上传{{ scopeLabel }}文件解析后自动生成，或手动新增。
    </div>
    <div class="cat-list">
      <div v-for="(cat, idx) in categories" :key="cat.id" class="cat-item">
        <div class="cat-item-head">
          <span class="cat-order">{{ idx + 1 }}</span>
          <span v-if="addingIndex === cat.id" class="cat-name">
            <t-input v-model="cat.name" size="small" placeholder="分类名称" @blur="commitAdd(cat)" />
          </span>
          <span v-else class="cat-name" :class="{ 'cat-disabled': !cat.enabled }" @click="toggleExpand(cat)">{{ cat.name }}</span>
          <div class="cat-actions">
            <t-switch v-model="cat.enabled" size="small" @change="saveCat(cat)" />
            <t-button variant="text" size="small" class="icon-btn" @click="toggleExpand(cat)">
              <t-icon :name="expanded.has(cat.id) ? 'chevron-up' : 'chevron-down'" size="14px" />
            </t-button>
            <t-popconfirm theme="warning" content="确定删除该分类吗？其下子项一并移除。" :confirm-btn="{ content: '删除', theme: 'danger' }"
              :cancel-btn="{ content: '取消' }" placement="top" @confirm="removeCat(cat)">
              <t-button variant="text" size="small" theme="danger" class="icon-btn">
                <t-icon name="delete" size="14px" />
              </t-button>
            </t-popconfirm>
          </div>
        </div>
        <transition name="cat-expand">
          <div v-if="expanded.has(cat.id)" class="cat-subs">
            <div class="cat-subs-head">
              <span class="cat-subs-title">子项</span>
            </div>
            <div v-if="!cat.subs?.length" class="sub-empty">暂无子项，上传解析后自动提取</div>
            <div v-for="sub in cat.subs || []" :key="sub.name" class="sub-row">
              <span class="sub-name">{{ sub.name }}</span>
              <t-switch v-model="sub.enabled" size="small" @change="saveCat(cat)" />
            </div>
          </div>
        </transition>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { listFleetCategories, createFleetCategory, updateFleetCategory, deleteFleetCategory } from '@/api/fleet'

const props = defineProps<{ scope: string }>()
const scopeLabel = ref(props.scope === 'vehicle' ? '车辆证照' : props.scope === 'driver' ? '司机证照' : '维保类型')

const categories = ref<any[]>([])
const expanded = ref<Set<string>>(new Set())
const addingIndex = ref('')

async function load() {
  try {
    const res = await listFleetCategories({ scope: props.scope })
    categories.value = res.data || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || '分类加载失败')
  }
}
onMounted(load)

function toggleExpand(cat: any) {
  if (expanded.value.has(cat.id)) expanded.value.delete(cat.id)
  else expanded.value.add(cat.id)
  expanded.value = new Set(expanded.value)
}

function startAdd() {
  const tmp: any = { id: '__new__', name: '', subs: [], enabled: true }
  addingIndex.value = tmp.id
  categories.value.unshift(tmp)
}

async function commitAdd(cat: any) {
  if (addingIndex.value !== cat.id) return
  addingIndex.value = ''
  const name = (cat.name || '').trim()
  if (!name) {
    categories.value = categories.value.filter((c: any) => c.id !== '__new__')
    return
  }
  try {
    const res = await createFleetCategory({ scope: props.scope, name, subs: [] })
    const created = res.data || res
    categories.value = categories.value.map((c: any) => (c.id === '__new__' ? created : c))
    MessagePlugin.success('分类已创建')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '创建失败')
    categories.value = categories.value.filter((c: any) => c.id !== '__new__')
  }
}

async function saveCat(cat: any) {
  try {
    await updateFleetCategory(cat.id, { name: cat.name, subs: cat.subs || [], enabled: cat.enabled })
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  }
}

async function removeCat(cat: any) {
  try {
    await deleteFleetCategory(cat.id)
    categories.value = categories.value.filter((c: any) => c.id !== cat.id)
    expanded.value.delete(cat.id)
    MessagePlugin.success('已删除')
  } catch (e: any) {
    MessagePlugin.error(e?.message || '删除失败')
  }
}
</script>

<style lang="less" scoped>
.fleet-cat-panel { display: flex; flex-direction: column; gap: 12px; }
.panel-head { display: flex; align-items: center; justify-content: space-between; }
.panel-title { font-size: 14px; font-weight: 600; color: var(--td-text-color-primary); }
.panel-empty { padding: 24px 0; text-align: center; color: var(--td-text-color-placeholder); font-size: 13px; }
.cat-list { display: flex; flex-direction: column; gap: 8px; }
.cat-item { border: 1px solid var(--td-component-border); border-radius: 8px; overflow: hidden; background: var(--td-bg-color-container); }
.cat-item-head { display: flex; align-items: center; gap: 10px; padding: 10px 12px; }
.cat-order { width: 20px; height: 20px; border-radius: 50%; background: var(--td-brand-color-light); color: var(--td-brand-color); font-size: 12px; display: flex; align-items: center; justify-content: center; flex: none; }
.cat-name { flex: 1; font-size: 14px; cursor: pointer; }
.cat-disabled { color: var(--td-text-color-placeholder); }
.cat-actions { display: flex; align-items: center; gap: 4px; }
.icon-btn { width: 28px; height: 28px; padding: 0; display: flex; align-items: center; justify-content: center; }
.cat-subs { border-top: 1px dashed var(--td-component-border); padding: 10px 12px; background: var(--td-bg-color-container-hover); }
.cat-subs-head { display: flex; align-items: center; margin-bottom: 8px; }
.cat-subs-title { font-size: 12px; color: var(--td-text-color-placeholder); }
.sub-empty { font-size: 12px; color: var(--td-text-color-placeholder); padding: 4px 0; }
.sub-row { display: flex; align-items: center; justify-content: space-between; padding: 6px 4px; border-bottom: 1px solid var(--td-component-border); }
.sub-row:last-child { border-bottom: none; }
.sub-name { font-size: 13px; color: var(--td-text-color-primary); }
.cat-expand-enter-active, .cat-expand-leave-active { transition: opacity .18s ease; }
.cat-expand-enter-from, .cat-expand-leave-to { opacity: 0; }
</style>
