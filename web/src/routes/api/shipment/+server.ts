import { json, type RequestHandler } from '@sveltejs/kit';

export const POST: RequestHandler = async ({ request }) => {
	const { signal, shipment } = await request.json();

	try {
		const response = await fetch(
			`http://localhost:8081/shipments/${shipment.id}/status`,
			{
				method: 'POST',
				body: JSON.stringify({ status: signal.status })
			}
		);
		return json({ status: 'ok', body: response });
	} catch (e) {
		return json({ status: 'error' });
	}
};
