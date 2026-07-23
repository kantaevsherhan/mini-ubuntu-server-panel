import { createApp } from 'vue'
import { createPinia } from 'pinia'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import Aura from '@primeuix/themes/aura'
import 'primeicons/primeicons.css'
import './styles/main.css'
import App from './App.vue'
import router from './router'

const app = createApp(App)
app
  .use(createPinia())
  .use(router)
  .use(PrimeVue, {
    theme: {
      preset: Aura,
      options: {
        darkModeSelector: '[data-theme="dark"]',
      },
    },
  })
  .use(ToastService)
  .use(ConfirmationService)
  .mount('#app')
