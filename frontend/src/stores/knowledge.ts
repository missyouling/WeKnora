import { ref } from "vue";

import { defineStore } from "pinia";

export const knowledgeStore = defineStore("knowledge", {
  state: () => ({
    cardList: ref<any[]>([]),
    total: ref<number>(0),
  }),
  actions: {},
});
