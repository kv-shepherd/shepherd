import { http, HttpResponse } from 'msw';

const API_BASE = 'http://localhost:8080/api/v1';

export const handlers = [
	// Health check for smoke/integration tests.
	http.get(`${API_BASE}/health/live`, () => {
		return HttpResponse.json({ status: 'ok' });
	}),

	// Notification badge baseline.
	http.get(`${API_BASE}/notifications/unread-count`, () => {
		return HttpResponse.json({ count: 0 });
	}),

	// Empty notification list baseline.
	http.get(`${API_BASE}/notifications`, () => {
		return HttpResponse.json({
			items: [],
			page: 1,
			per_page: 10,
			total: 0,
			total_pages: 0,
		});
	}),
];

