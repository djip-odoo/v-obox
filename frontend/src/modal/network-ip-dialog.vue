<template>
  <div class="mt-6 text-center">
    <div @click="showAddDialog = true"
      class="border-2 border-dashed border-gray-300 bg-gray-50 rounded-lg px-4 py-3 text-gray-600 hover:border-gray-400 hover:bg-gray-100 cursor-pointer">
      + Add Network Printer
    </div>
  </div>
  <teleport to="body">
    <transition enter-active-class="transition duration-200 ease-out" enter-from-class="opacity-0"
      enter-to-class="opacity-100" leave-active-class="transition duration-150 ease-in" leave-from-class="opacity-100"
      leave-to-class="opacity-0">
      <div v-if="showAddDialog" class="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4">
        <div class="absolute inset-0 bg-black/75" @click="onNetworkDialogClose(false)" />

        <div class="relative bg-white rounded-2xl w-full max-w-sm shadow-xl overflow-hidden p-6">
          <div class="flex items-center justify-between mb-4">
            <div class="text-lg font-medium">Add Network Printer</div>
            <CloseButton @click="onNetworkDialogClose(false)" />
          </div>

          <div class="flex items-center justify-center gap-2 mb-3" @paste.capture="onPaste">
            <template v-for="(_, index) in ipParts" :key="index">
              <input :ref="(el) => { if (el) inputRefs[index] = el }" v-model="ipParts[index]" type="text" inputmode="numeric"
                maxlength="3"
                class="w-16 border border-gray-300 rounded-lg px-2 py-2 text-center text-sm focus:outline-none focus:ring-1 focus:ring-odoo-light focus:border-transparent"
                @input="onInput(index)" @keydown="onKeyDown($event, index)" @keyup.enter="submit" />

              <span v-if="index < 3" class="text-gray-400 font-medium">
                .
              </span>
            </template>
          </div>

          <div v-if="error" class="text-danger text-sm mb-3">
            {{ error }}
          </div>

          <button @click="submit" :disabled="loading"
            class="w-full border rounded-lg px-4 py-2 cursor-pointer text-sm bg-odoo text-white hover:bg-odoo-dark disabled:opacity-50 disabled:cursor-not-allowed">
            {{ loading ? 'Adding...' : 'Add' }}
          </button>
        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup>
import { ref, watch, nextTick } from 'vue'
import CloseButton from './close-button.vue'
import { AddLANPrinter } from '../../wailsjs/go/main/App'

const emit = defineEmits(['notify', 'refresh'])
const ipParts = ref(['', '', '', ''])
const showAddDialog = ref(false)
const error = ref(null)
const loading = ref(false)
const inputRefs = ref([])

watch(showAddDialog, (val) => {
  if (val) {
    ipParts.value = ['', '', '', '']
    error.value = null
    inputRefs.value = []
    nextTick(() => inputRefs.value[0]?.focus())
  }
}
)

function onInput(index) {
  let value = ipParts.value[index]
  value = value.replace(/\D/g, '')
  value = value.slice(0, 3)
  ipParts.value[index] = value
  if ((value.length === 3 || (value.length > 1 && Number(value) > 25)) && index < 3) {
    nextTick(() => {
      inputRefs.value[index + 1]?.focus()
      inputRefs.value[index + 1]?.select?.()
    })
  }
  if (ipParts.value.every((part) => part)) {
    error.value = null
  }
}

function onKeyDown(event, index) {
  if (event.ctrlKey || event.metaKey) return;

  const allowedKeys = ['Backspace', 'Delete', 'ArrowLeft', 'ArrowRight', 'Tab']
  if (!/[0-9]/.test(event.key) && !allowedKeys.includes(event.key)) {
    if (event.key === '.') {
      event.preventDefault()

      if (index < 3) {
        nextTick(() => inputRefs.value[index + 1]?.focus())
      }
      return
    }
    event.preventDefault()
  }

  if (event.key === 'Backspace' && !ipParts.value[index] && index > 0) {
    nextTick(() => inputRefs.value[index - 1]?.focus())
  }
}

function onPaste(event) {
  const pasted = event.clipboardData?.getData('text')?.trim()
  if (!pasted) {
    error.value = 'Please enter a valid IP address'
    return
  }

  const ip = extractIP(pasted)
  if (!ip) {
    error.value = 'Please enter a valid IP address'
    return
  }

  const parts = ip.split('.')
  if (parts.length !== 4) {
    error.value = `Please enter a valid IP address`
    return
  }

  if (parts.some((p) => !isValidOctet(p))) {
    error.value = 'Each octet must be between 0 and 255'
    return
  }

  event.preventDefault()
  ipParts.value = [...parts]
  error.value = null
  nextTick(() => inputRefs.value[3]?.focus())
}

function isValidOctet(val) {
  const n = Number(val)
  return Number.isInteger(n) && n >= 0 && n <= 255
}

function extractIP(text) {
  const match = text.match(/\b(?:\d{1,3}\.){3}\d{1,3}\b/)
  return match ? match[0] : null
}

async function submit() {
  if (ipParts.value.some((part) => !part)) {
    error.value = 'Please enter a valid IP address'
    return
  }

  if (ipParts.value.some((part) => !isValidOctet(part))) {
    error.value = 'Each octet must be between 0 and 255'
    return
  }

  const ip = ipParts.value.join('.')
  loading.value = true
  error.value = null
  try {
    await AddLANPrinter(ip)
    emit('notify', 'Printer added successfully', 'success')
    onNetworkDialogClose(true)
  } catch (err) {
    console.error(err)
    emit('notify', err || 'Failed to add printer', 'danger')
    error.value = err || 'Failed to add printer'
  } finally {
    loading.value = false
  }
}

function onNetworkDialogClose(shouldRefresh) {
  error.value = null
  showAddDialog.value = false
  if (shouldRefresh) emit('refresh')
}

</script>
