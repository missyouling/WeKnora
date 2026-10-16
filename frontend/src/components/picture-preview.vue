<script setup lang="ts">

defineProps(['reviewImg', 'reviewUrl'])
const emit = defineEmits(['closePreImg'])
const close = () => {
    emit('closePreImg')
}
</script>
<template>
    <t-image-viewer :visible="reviewImg" close-on-overlay close-on-esc-keydown @close="close"
        :images="reviewUrl ? [reviewUrl] : []">
    </t-image-viewer>
</template>
<style scoped lang="less"></style>
