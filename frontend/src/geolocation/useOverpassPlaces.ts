import { Place } from '../places/places'

interface OverpassElement {
  type: string
  id: number
  lat: number
  lon: number
  tags: Record<string, string>
}

interface OverpassResponse {
  elements: OverpassElement[]
}

const OVERPASS_URL = 'https://overpass-api.de/api/interpreter'

function buildQuery(lat: number, lon: number, radius: number): string {
  return `
    [out:json][timeout:20];
    (
      node["tourism"](around:${radius},${lat},${lon});
      node["historic"](around:${radius},${lat},${lon});
      node["amenity"="museum"](around:${radius},${lat},${lon});
      node["amenity"="theatre"](around:${radius},${lat},${lon});
      node["amenity"="cafe"](around:${radius},${lat},${lon});
      node["amenity"="restaurant"](around:${radius},${lat},${lon});
    );
    out center;
  `
}

function elementToPlace(el: OverpassElement): Place | null {
  const tags = el.tags
  const name = tags.name
  if (!name) return null

  const tourism = tags.tourism
  const historic = tags.historic
  const amenity = tags.amenity

  let category = 'достопримечательность'
  if (tourism) category = tourism
  else if (historic) category = historic
  else if (amenity) category = amenity

  return {
    id: `${el.type}/${el.id}`,
    name,
    description: tags.description || tags['short_name'] || '',
    latitude: el.lat,
    longitude: el.lon,
    category,
    images: [],
  }
}

export async function fetchNearbyPlaces(
  lat: number,
  lon: number,
  radius = 1000
): Promise<Place[]> {
  const query = buildQuery(lat, lon, radius)
  const response = await fetch(OVERPASS_URL, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ data: query }),
  })

  if (!response.ok) throw new Error(`Overpass API error: ${response.status}`)

  const data: OverpassResponse = await response.json()
  const places: Place[] = []

  for (const el of data.elements) {
    const place = elementToPlace(el)
    if (place) places.push(place)
  }

  return places
}
