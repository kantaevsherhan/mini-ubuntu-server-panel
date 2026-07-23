import { computed } from 'vue'
import { storeToRefs } from 'pinia'
import { usePreferencesStore } from '../stores/preferences'
const messages={
 ru:{dashboard:'Обзор',docker:'Docker',processes:'Процессы',services:'Сервисы',terminal:'Терминал',files:'Файлы',users:'Пользователи',firewall:'Firewall',logs:'Логи',audit:'Аудит',notifications:'Уведомления',settings:'Настройки',online:'Онлайн',welcome:'Состояние сервера',panelUsers:'Пользователи панели',pending:'Ожидают отправки',theme:'Тема',language:'Язык',appearance:'Интерфейс',logout:'Выйти',login:'Войти',username:'Имя пользователя',password:'Пароль',server:'Сервер'},
 en:{dashboard:'Dashboard',docker:'Docker',processes:'Processes',services:'Services',terminal:'Terminal',files:'Files',users:'Users',firewall:'Firewall',logs:'Logs',audit:'Audit',notifications:'Notifications',settings:'Settings',online:'Online',welcome:'Server status',panelUsers:'Panel users',pending:'Pending delivery',theme:'Theme',language:'Language',appearance:'Appearance',logout:'Sign out',login:'Sign in',username:'Username',password:'Password',server:'Server'}
} as const
export function useI18n(){const{locale}=storeToRefs(usePreferencesStore());const t=computed(()=>messages[locale.value]);return{t,locale}}
