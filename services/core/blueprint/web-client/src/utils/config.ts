export let BASE_URL = '';
export let RAFT_BASE_URL = '';
export let LOCAL_DOMAIN = 'localhost';

if (process.env.NODE_ENV === 'development') {
  BASE_URL = `http://${LOCAL_DOMAIN}:2221`; }


if (process.env.NODE_ENV === 'development') {
  RAFT_BASE_URL = `http://${LOCAL_DOMAIN}:1111`; }