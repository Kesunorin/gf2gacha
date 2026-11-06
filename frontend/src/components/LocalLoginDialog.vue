<script setup lang="ts">
import {ref} from "vue";
import {ReadLocalLoginInfo} from "../../wailsjs/go/main/App";
import {model} from "../../wailsjs/go/models.ts";

const visible = defineModel({type: Boolean, required: true})
const emit = defineEmits<{ (e: 'success', info: model.LogInfo): void }>()

const reading = ref(false)
const errorMessage = ref('')

const read = async () => {
  reading.value = true
  errorMessage.value = ''
  try {
    const info = await ReadLocalLoginInfo()
    emit('success', info)
    visible.value = false
  } catch (err) {
    errorMessage.value = String(err)
  } finally {
    reading.value = false
  }
}

</script>

<template>
  <el-dialog v-model="visible" width="600" destroy-on-close>
    <template #header>
      <div class="text-xl font-bold">读取本地登录信息</div>
    </template>
    <div class="flex flex-col gap-1">
      <p class="text-green-600">读取游戏客户端本地保存的登录凭证，不抓包、不装证书、不用打开招募界面</p>
      <p class="mb-2">令牌只在本地解密，仅用于向官方接口换取抽卡记录</p>
      <p>1.确认游戏至少登录过一次（不要求游戏正在运行）</p>
      <p>2.点击下方按钮</p>
      <p class="mt-2 text-orange-500">若提示令牌已过期，启动一次游戏重新登录后再试即可</p>
      <el-alert v-if="errorMessage" class="mt-2" :title="errorMessage" type="error" show-icon :closable="false"></el-alert>
      <div class="mt-4">
        <el-button type="success" class="font-bold" :loading="reading" @click="read">读取</el-button>
      </div>
    </div>
  </el-dialog>
</template>
