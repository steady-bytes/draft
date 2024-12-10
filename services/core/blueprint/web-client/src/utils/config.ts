export let BASE_URL = '';
export let LOCAL_DOMAIN = 'localhost';

if (process.env.NODE_ENV === 'development') {
  BASE_URL = `http://${LOCAL_DOMAIN}:3331`; } 