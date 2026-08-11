<template>
  <div class="mt-3 flex items-center justify-center gap-2">
    <div class="flex items-center gap-2">
      <span class="text-xs font-medium text-gray-700">
        Allow Other Devices to Print
      </span>

      <button
        @click="toggleFirewall"
        type="button"
        :class="[
          isFirewallEnabled ? 'bg-odoo' : 'bg-gray-300',
          'relative inline-flex h-4 w-7 items-center rounded-full transition-colors duration-200 focus:outline-none cursor-pointer shrink-0'
        ]"
      >
        <span :class="[
          isFirewallEnabled ? 'translate-x-4' : 'translate-x-0.5',
          'inline-block h-3 w-3 transform rounded-full bg-white transition duration-200'
        ]" />
      </button>

      <div class="relative flex items-center group">
        <svg
          class="w-3.5 h-3.5 text-gray-400 hover:text-gray-600 transition-colors cursor-help shrink-0"
          fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"
        >
          <path stroke-linecap="round" stroke-linejoin="round"
            d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <div
          class="absolute bottom-full left-1/2 -translate-x-1/2 mb-1.5 hidden group-hover:block w-48 bg-gray-900/95 text-white text-[10px] rounded py-1 px-2 shadow-md text-center leading-normal z-50 pointer-events-none"
        >
          If the port changes, this setting will be reset.
          <div class="absolute top-full left-1/2 -translate-x-1/2 border-4 border-transparent border-t-gray-900/95" />
        </div>
      </div>
    </div>
  </div>

  <!-- Firewall dialog (enable or disable confirmation) -->
  <FirewallEnableSettingDialog
    v-if="showDialog"
    :os="os"
    :mode="dialogMode"
    @notify="(msg, type) => emit('notify', msg, type)"
    @refresh="onDialogRefresh"
    @close="onDialogClose"
  />
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { IsFirewallEnabled } from '../../wailsjs/go/main/App'
import FirewallEnableSettingDialog from './firewall-enable-setting-dialog.vue'

const props = defineProps({
  os: String,
})

const emit = defineEmits(['notify', 'refresh'])

const isFirewallEnabled = ref(false)
const os = ref("darwin")
const showDialog = ref(false)
const dialogMode = ref('enable')  // 'enable' | 'disable'

onMounted(async () => {
  await fetchStatus()
})

async function fetchStatus() {
  try {
    const res = await IsFirewallEnabled()
    // os.value = res.os
    isFirewallEnabled.value = res.isFirewallEnabled
  } catch (err) {
    emit('notify', err || 'Failed to retrieve firewall status', 'danger')
  }
}

function toggleFirewall() {
  if (isFirewallEnabled.value) {
    // Show disable-warning dialog before actually disabling
    dialogMode.value = 'disable'
    showDialog.value = true
  } else {
    // Show enable-confirmation dialog
    dialogMode.value = 'enable'
    showDialog.value = true
  }
}

function onDialogRefresh() {
  // Dialog completed its action (enable or disable succeeded)
  isFirewallEnabled.value = dialogMode.value === 'enable'
  showDialog.value = false
  emit('refresh')
}

function onDialogClose() {
  // User cancelled — toggle stays as-is
  showDialog.value = false
}
</script>
