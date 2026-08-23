import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 50,
  duration: '30s',
};

export function setup() {
  const url = `${__ENV.API_URL || 'http://localhost:8080'}`;
  const registerPayload = JSON.stringify({
    email: `loadtest-${Date.now()}@obsidian.io`,
    password: 'password123',
  });
  
  const headers = { 'Content-Type': 'application/json' };
  
  let res = http.post(`${url}/api/auth/register`, registerPayload, { headers });
  let token = '';
  
  if (res.status === 201) {
    token = JSON.parse(res.body).token;
  } else {
    // If user already exists, try logging in
    const loginPayload = JSON.stringify({
      email: 'loadtest@obsidian.io',
      password: 'password123',
    });
    res = http.post(`${url}/api/auth/login`, loginPayload, { headers });
    token = JSON.parse(res.body).token;
  }

  return { token };
}

export default function (data) {
  const url = `${__ENV.API_URL || 'http://localhost:8080'}`;
  const queueId = `${__ENV.QUEUE_ID}`;
  
  const payload = JSON.stringify({
    job_type: 'noop',
    payload: {},
    priority: 1,
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${data.token}`,
    },
  };

  const res = http.post(`${url}/api/queues/${queueId}/jobs`, payload, params);
  
  check(res, {
    'status is 201': (r) => r.status === 201,
  });
  
  sleep(0.05); // Throttle lightly per virtual user
}
