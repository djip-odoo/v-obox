<template>
  <button @click="openSettings" title="Customer Display"
    class="absolute top-3 left-16 w-12 h-12 flex items-center justify-center rounded-full bg-odoo text-white shadow-lg hover:scale-105 transition-all duration-200">
    <div class="relative">
      <svg class="w-6 h-6" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M9.75 17L9 20m6-3l.75 3M4 5h16v10H4V5zm0 10h16" />
      </svg>
      <span v-if="isOpenOnDesktop"
        class="absolute -top-1 -right-1 w-2.5 h-2.5 rounded-full bg-green-400 border border-white animate-pulse" />
    </div>
  </button>

  <teleport to="body">
    <transition enter-active-class="transition duration-200 ease-out" enter-from-class="opacity-0"
      enter-to-class="opacity-100" leave-active-class="transition duration-150 ease-in" leave-from-class="opacity-100"
      leave-to-class="opacity-0">
      <div v-if="show" class="fixed inset-0 flex items-end sm:items-center justify-center p-4"
        :style="{ zIndex: zIndex }">
        <div class="absolute inset-0 bg-black/60 backdrop-blur-sm" @click="close" />

        <div class="relative bg-white rounded-2xl w-full max-w-md shadow-2xl overflow-hidden flex flex-col"
          style="max-height: 85vh;">

          <!-- Header -->
          <div class="bg-odoo px-5 py-4 flex items-center justify-between flex-shrink-0">
            <div class="flex items-center gap-3">
              <div class="w-8 h-8 rounded-lg bg-white/20 flex items-center justify-center">
                <svg class="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round"
                    d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
                </svg>
              </div>
              <div>
                <p class="text-white font-semibold text-sm leading-tight">Customer Display</p>
                <p class="text-white/60 text-xs">WebView Configuration</p>
              </div>
            </div>
            <CloseButton @click="close" />
          </div>

          <!-- Body -->
          <div class="flex-1 overflow-y-auto p-5 space-y-4">

            <!-- Loading -->
            <div v-if="loading" class="flex items-center justify-center py-10 gap-2">
              <svg class="w-4 h-4 text-odoo animate-spin" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"></path>
              </svg>
              <span class="text-sm text-gray-500">Loading...</span>
            </div>

            <template v-else>

              <!-- Add URL form (shown when no URL exists) -->
              <transition mode="out-in" enter-active-class="transition-all duration-200 ease-out"
                enter-from-class="opacity-0 scale-95 translate-y-2" enter-to-class="opacity-100 scale-100 translate-y-0"
                leave-active-class="transition-all duration-150 ease-in"
                leave-from-class="opacity-100 scale-100 translate-y-0"
                leave-to-class="opacity-0 scale-95 -translate-y-2">
                <div>
                  <div v-if="currentURL"
                    class="relative overflow-hidden rounded-2xl border border-green-200 bg-gradient-to-br from-green-50 to-white shadow-sm">
                    <!-- Status ribbon -->
                    <div
                      class="absolute top-0 right-0 px-3 py-1 bg-green-500 text-white text-[10px] font-bold uppercase tracking-wider">
                      Active
                    </div>

                    <div class="p-5">

                      <div class="flex items-start gap-4">

                        <div class="w-12 h-12 rounded-xl bg-green-100 flex items-center justify-center flex-shrink-0">
                          <svg class="w-6 h-6 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                              d="M4 5h16v10H4V5zm6 12h4" />
                          </svg>
                        </div>

                        <div class="flex-1 min-w-0">

                          <div class="flex items-center gap-2 mb-1">
                            <div class="w-2 h-2 rounded-full bg-green-500 animate-pulse"></div>
                            <span class="text-xs font-semibold text-green-700 uppercase tracking-wide">
                              Customer Display Online
                            </span>
                          </div>

                          <h3 class="text-base font-semibold text-gray-900 truncate">
                            {{ currentURL.name || currentURLHostname }}
                          </h3>
                        </div>
                      </div>

                      <div class="grid grid-cols-2 gap-3 mt-5">

                        <!-- Launch / Close control of desktop display -->
                        <button @click="toggleDesktopDisplay(!isOpenOnDesktop)"
                          class="flex items-center justify-center gap-2 rounded-xl py-2.5 font-medium hover:opacity-90 transition-all"
                          :class="isOpenOnDesktop ? 'bg-amber-600 text-white' : 'bg-odoo text-white'">
                          <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                            <path v-if="isOpenOnDesktop" stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                              d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
                            <path v-else stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                              d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                          </svg>
                          {{ isOpenOnDesktop ? 'Close' : 'Launch' }}
                        </button>

                        <button @click="deleteURL" :disabled="deleteLoading"
                          class="flex items-center justify-center gap-2 rounded-xl border border-red-200 bg-red-50 text-red-600 py-2.5 font-medium hover:bg-red-100 transition-all">
                          Delete
                        </button>

                      </div>

                      <!-- Monitor Selection Section (Desktop App Only) -->
                      <div v-if="isDesktopApp()" class="border-t border-gray-100 mt-6 pt-5 space-y-4">
                        <div class="flex items-center justify-between">
                          <h4 class="text-xs font-semibold text-gray-800 uppercase tracking-wider">
                            Monitor Settings
                          </h4>
                          <button @click="identifyDisplays"
                            class="text-xs font-medium text-odoo hover:underline flex items-center gap-1 cursor-pointer">
                            <svg class="w-3.5 h-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                                d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                                d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                            </svg>
                            Identify Displays
                          </button>
                        </div>

                        <!-- Monitors List -->
                        <div class="space-y-2">
                          <div v-for="monitor in monitors" :key="monitor.id"
                            @click="selectMonitor(monitor.id)" class="relative overflow-hidden rounded-xl border p-4 transition-all duration-200" :class="[
                              selectedMonitorID === monitor.id
                                ? 'border-odoo bg-odoo/5 shadow-sm'
                                : 'border-gray-200 hover:border-gray-300 hover:bg-gray-50/50'
                            ]">

                            <div class="flex items-center justify-between">
                              <div class="flex items-center gap-3">
                                <!-- Monitor Icon -->
                                <div class="w-8 h-8 rounded-lg flex items-center justify-center"
                                  :class="selectedMonitorID === monitor.id ? 'bg-odoo/10 text-odoo' : 'bg-gray-100 text-gray-500'">
                                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                                      d="M9.75 17L9 20m6-3l.75 3M4 5h16v10H4V5zm0 10h16" />
                                  </svg>
                                </div>
                                <div>
                                  <p class="text-sm font-semibold"
                                    :class="selectedMonitorID === monitor.id ? 'text-odoo' : 'text-gray-800'">
                                    {{ monitor.name }}
                                    <span v-if="monitor.isPrimary"
                                      class="ml-1.5 inline-flex items-center px-2 py-0.5 rounded-full text-[10px] font-medium bg-gray-100 text-gray-600 border border-gray-200">
                                      Primary
                                    </span>
                                  </p>
                                  <p class="text-xs text-gray-500">
                                    {{ monitor.width }} × {{ monitor.height }}
                                  </p>
                                </div>
                              </div>

                              <!-- Radio Icon -->
                              <div class="w-5 h-5 rounded-full border flex items-center justify-center transition-all"
                                :class="selectedMonitorID === monitor.id ? 'border-odoo bg-odoo text-white' : 'border-gray-300'">
                                <div v-if="selectedMonitorID === monitor.id" class="w-2 h-2 rounded-full bg-white">
                                </div>
                              </div>
                            </div>
                          </div>

                          <div v-if="monitors.length === 0" class="text-center py-6 text-sm text-gray-400">
                            No connected monitors detected.
                          </div>
                        </div>

                        <!-- Remember my selection -->
                        <div class="flex items-center gap-2.5">
                          <input type="checkbox" id="remember-monitor" v-model="rememberSelection"
                            @change="saveMonitorSettings"
                            class="w-4 h-4 rounded border-gray-300 text-odoo focus:ring-odoo transition-all" />
                          <label for="remember-monitor" class="text-xs text-gray-600 cursor-pointer select-none">
                            Remember my selection
                          </label>
                        </div>

                        <!-- Test button -->
                        <div class="pt-2">
                          <button @click="testDisplay" :disabled="!selectedMonitorID"
                            class="w-full flex items-center justify-center gap-2 rounded-xl border border-gray-200 bg-gray-50 text-gray-700 py-2.5 text-sm font-medium hover:bg-gray-100 disabled:opacity-50 disabled:cursor-not-allowed transition-all">
                            <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                                d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                            </svg>
                            Test Customer Display
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div v-else-if="!openForm"
                    class="rounded-2xl border-2 border-dashed border-gray-200 bg-gradient-to-b from-gray-50 to-white p-8 text-center cursor-pointer"
                    @click="openForm = true">
                    <div class="w-16 h-16 mx-auto rounded-2xl bg-gray-100 flex items-center justify-center mb-4">
                      <svg class="w-8 h-8 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                          d="M4 5h16v10H4V5zm6 12h4" />
                      </svg>
                    </div>

                    <h3 class="text-base font-semibold text-gray-800">
                      Customer Display Not Configured
                    </h3>

                    <p class="text-sm text-gray-500 mt-2 max-w-xs mx-auto">
                      Configure a web URL below to display your customer-facing screen in kiosk mode.
                    </p>
                    <p class="text-sm text-gray-500 mt-2 max-w-xs mx-auto cursor-pointer">
                      click to configure
                    </p>

                    <div
                      class="inline-flex items-center gap-2 mt-4 px-3 py-1.5 rounded-full bg-amber-50 text-amber-700 text-xs font-medium">
                      <div class="w-2 h-2 rounded-full bg-amber-500"></div>
                      Setup Required
                    </div>
                  </div>
                  <div v-if="!currentURL && openForm"
                    class="rounded-2xl border border-odoo/15 bg-white shadow-sm overflow-hidden">

                    <!-- Header -->
                    <div class="px-5 py-4 bg-gradient-to-r from-odoo/10 to-odoo/5 border-b border-odoo/10">
                      <div class="flex items-center gap-3">
                        <div class="w-10 h-10 rounded-xl bg-odoo text-white flex items-center justify-center">
                          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                            <path stroke-linecap="round" stroke-linejoin="round" d="M4 5h16v10H4V5zm6 12h4" />
                          </svg>
                        </div>

                        <div>
                          <h3 class="font-semibold text-gray-900">
                            Customer Display
                          </h3>
                          <p class="text-xs text-gray-500">
                            Configure the webpage shown on the customer display
                          </p>
                        </div>
                        <CloseButton @click="openForm = false" />
                      </div>
                    </div>

                    <!-- Body -->
                    <div class="p-5 space-y-4">

                      <div>
                        <label class="block text-xs font-semibold text-gray-700 mb-2">
                          Display URL
                        </label>

                        <input id="cd-url" v-model="form.url" type="url" placeholder="https://display.example.com"
                          class="w-full rounded-xl border border-gray-300 px-4 py-3 text-sm font-mono focus:ring-2 focus:ring-odoo focus:border-odoo outline-none transition-all"
                          :class="{ 'border-danger ring-1 ring-danger': formErrors.url }" @input="validateURL" />

                        <p v-if="formErrors.url" class="text-xs text-danger mt-2">
                          {{ formErrors.url }}
                        </p>
                      </div>

                      <!-- Preview -->
                      <div v-if="urlValid && form.url" class="rounded-xl border border-green-200 bg-green-50 p-4">
                        <div class="flex items-center gap-2 mb-2">
                          <div class="w-2 h-2 rounded-full bg-green-500"></div>
                          <span class="text-xs font-semibold text-green-700 uppercase tracking-wide">
                            Display Preview
                          </span>
                        </div>

                        <p class="text-sm font-semibold text-gray-900">
                          {{ formURLHostname }}
                        </p>

                        <p class="text-xs text-green-700 font-mono break-all mt-1">
                          {{ form.url }}
                        </p>
                      </div>

                      <div v-if="formError"
                        class="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-xs text-red-600">
                        {{ formError }}
                      </div>

                      <button @click="addURL" :disabled="formLoading"
                        class="w-full rounded-xl bg-odoo text-white py-3 font-semibold hover:opacity-90 disabled:opacity-50 disabled:cursor-not-allowed transition-all">
                        {{ formLoading ? 'Saving...' : 'Save & Activate' }}
                      </button>

                    </div>
                  </div>
                </div>
              </transition>

            </template>
          </div>

          <!-- Footer -->
          <div class="px-5 py-3 border-t border-gray-100 flex-shrink-0">
            <button @click="close"
              class="w-full border border-gray-200 rounded-xl px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-50 transition-colors cursor-pointer">
              Close
            </button>
          </div>

        </div>
      </div>
    </transition>
  </teleport>
</template>

<script setup>
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import CloseButton from './close-button.vue'
import { connector, safeEventsOn, isDesktopApp } from '../connector'
import { useToast } from '../hooks/useToast'

const props = defineProps({
  // When rendered inside the WebView overlay, pass a higher z-index
  zIndex: { type: Number, default: 150 }
})

const emit = defineEmits(['open-webview', 'close'])
const { notify } = useToast()

// Safe URL hostname helpers — prevent TypeError when url is undefined/invalid
const currentURLHostname = computed(() => {
  try { return new URL(currentURL.value?.url ?? '').hostname } catch { return currentURL.value?.url ?? '' }
})
const formURLHostname = computed(() => {
  try { return new URL(form.value.url).hostname.replace(/^www\./, '') } catch { return form.value.url }
})

const show = ref(false)
const loading = ref(false)
const currentURL = ref(null)   // The single configured URL (or null)
const isOpenOnDesktop = ref(false) // Open state on desktop application

// Delete
const deleteLoading = ref(false)

// Form state
const openForm = ref(false)
const formLoading = ref(false)
const formError = ref(null)
const urlValid = ref(false)
const form = ref({ name: '', url: '', description: '' })
const formErrors = ref({ name: '', url: '' })

// Monitor selection state
const monitors = ref([])
const selectedMonitorID = ref('')
const rememberSelection = ref(false)

// ── Lifecycle ─────────────────────────────────────────────────────────────────

let statusInterval = null

onMounted(() => {
  safeEventsOn('open-customer-display-settings', () => openSettings())

  if (isDesktopApp()) {
    safeEventsOn('monitors-changed', (newMonitors) => {
      monitors.value = newMonitors
      notify('Monitor configuration updated', 'info')
    })

    safeEventsOn('selected-monitor-disconnected', () => {
      isOpenOnDesktop.value = false
      notify('Selected monitor was disconnected. Customer display closed.', 'warning')
      openSettings()
    })

    safeEventsOn('customer-display-selection-required', () => {
      notify('Please configure a monitor for the Customer Display', 'warning')
      openSettings()
    })
  }

  // Initial status load
  loadURL()
  loadMonitorSettings()

  // Poll state every 5s to keep active display dot and control sync
  statusInterval = setInterval(() => {
    refreshOpenState()
  }, 5000)
})

onUnmounted(() => {
  if (statusInterval) clearInterval(statusInterval)
})

watch(() => show.value, (val) => {
  if (val) {
    loadURL()
    loadMonitorSettings()
    resetForm()
  }
})

// ── Data ──────────────────────────────────────────────────────────────────────

async function loadURL() {
  loading.value = true
  try {
    currentURL.value = await connector.getActiveCustomerDisplayURL()
    await refreshOpenState()
  } catch (err) {
    console.error('Failed to load customer display URL:', err)
    notify('Failed to load customer display URL', 'danger')
  } finally {
    loading.value = false
  }
}

async function refreshOpenState() {
  // Don't poll if connector uses token-based auth and we don't have a token yet
  if (typeof connector.token !== 'undefined' && !connector.token) return
  try {
    isOpenOnDesktop.value = await connector.isCustomerDisplayOpen()
  } catch (err) {
    // Swallow 401s silently — they occur before PIN auth in HTTP mode
    if (err?.code !== 'PIN_REQUIRED' && err?.code !== 'PIN_NOT_SET') {
      console.warn('Failed to fetch customer display status:', err)
    }
  }
}

// ── Monitor Settings ─────────────────────────────────────────────────────────

async function loadMonitorSettings() {
  if (!isDesktopApp()) return
  try {
    monitors.value = await connector.getMonitors()
    const selection = await connector.getMonitorSelection()
    if (selection) {
      selectedMonitorID.value = selection[0] || ''
      rememberSelection.value = selection[1] || false
    }
  } catch (err) {
    console.error('Failed to load monitor settings:', err)
  }
}

async function selectMonitor(id) {
  selectedMonitorID.value = id
  // Selecting a monitor implicitly enables "remember" — user wants this to persist
  rememberSelection.value = true
  await saveMonitorSettings()
}

async function saveMonitorSettings() {
  if (!isDesktopApp()) return
  try {
    await connector.saveMonitorSelection(selectedMonitorID.value, rememberSelection.value)
    // If display is currently open, move it to the newly selected monitor
    if (isOpenOnDesktop.value && selectedMonitorID.value && currentURL.value) {
      await connector.openCustomerDisplayWindow(selectedMonitorID.value, currentURL.value.url)
    }
  } catch (err) {
    console.error('Failed to save monitor selection:', err)
    notify(`Failed to save monitor selection: ${err?.message || String(err)}`, 'danger')
  }
}

async function identifyDisplays() {
  try {
    await connector.identifyDisplays()
    notify('Identifying monitors...', 'success')
  } catch (err) {
    console.error('Failed to identify displays:', err)
    notify('Failed to identify displays', 'danger')
  }
}

async function testDisplay() {
  if (!selectedMonitorID.value) return
  try {
    await connector.testCustomerDisplay(selectedMonitorID.value)
    notify('Diagnostic test launched on selected monitor', 'success')
  } catch (err) {
    console.error('Failed to run test:', err)
    notify('Failed to run test', 'danger')
  }
}

// ── URL Validation ─────────────────────────────────────────────────────────────

function validateURL() {
  formErrors.value.url = ''
  urlValid.value = false
  const v = form.value.url.trim()
  if (!v) return
  try {
    const parsed = new URL(v)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      formErrors.value.url = 'URL must use http:// or https://'
      return
    }
    urlValid.value = true
  } catch {
    formErrors.value.url = 'Invalid URL format'
  }
}

function resetForm() {
  form.value = { name: 'Customer Display', url: '', description: '' }
  formErrors.value = { name: '', url: '' }
  formError.value = null
  urlValid.value = false
}

// ── Add ───────────────────────────────────────────────────────────────────────

async function addURL() {
  formErrors.value = { name: '', url: '' }
  formError.value = null

  if (!form.value.name.trim()) {
    formErrors.value.name = 'Name is required'
    return
  }

  validateURL()
  if (!urlValid.value) return

  formLoading.value = true
  try {
    // Add the URL and immediately set it as active
    const record = await connector.addCustomerDisplayURL(
      form.value.name.trim(),
      form.value.url.trim(),
      form.value.description.trim()
    )
    if (record?.id) {
      await connector.setActiveCustomerDisplayURL(record.id)
    }
    notify('Customer display URL saved', 'success')
    resetForm()
    await loadURL()
    await loadMonitorSettings()
  } catch (err) {
    formError.value = err?.message || 'Failed to save URL'
  } finally {
    formLoading.value = false
  }
}

// ── Delete ────────────────────────────────────────────────────────────────────

async function deleteURL() {
  if (!currentURL.value) return
  if (!window.confirm(`Delete "${currentURL.value.name}"?\n\nThe customer display WebView will be disabled.`)) return

  deleteLoading.value = true
  try {
    await connector.deleteCustomerDisplayURL(currentURL.value.id)
    notify('Customer display URL deleted', 'success')
    currentURL.value = null
  } catch (err) {
    notify(err?.message || 'Failed to delete URL', 'danger')
  } finally {
    deleteLoading.value = false
  }
}

// ── Open WebView ──────────────────────────────────────────────────────────────

function emitOpenWebView() {
  if (!currentURL.value) {
    notify('No URL configured', 'danger')
    return
  }
  emit('open-webview', currentURL.value.url)
  close()
}

async function toggleDesktopDisplay(open) {
  try {
    if (isDesktopApp()) {
      if (open && !selectedMonitorID.value) {
        notify('Please select a monitor before launching', 'warning')
        return
      }
    }
    await connector.setCustomerDisplayOpen(open)
    isOpenOnDesktop.value = open
    notify(open ? 'Customer display launched' : 'Customer display closed', 'success')
  } catch (err) {
    console.error('Failed to toggle customer display:', err)
    notify(err?.message || 'Failed to toggle customer display', 'danger')
  }
}

// ── Show / Hide ───────────────────────────────────────────────────────────────

function openSettings() {
  show.value = true
}

function close() {
  show.value = false
  emit('close')
}

// ── Expose ────────────────────────────────────────────────────────────────────
defineExpose({ openSettings, close, loadURL })
</script>
