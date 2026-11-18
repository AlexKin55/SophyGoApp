export interface Place {
  id: string
  name: string
  description: string
  latitude: number
  longitude: number
  category: string
  images: string[]
}

export const PLACES: Place[] = [
  {
    id: 'red-square',
    name: 'Красная площадь',
    description:
      'Центральная площадь Москвы, главная площадь России. Расположена в самом сердце города, у стен Московского Кремля.',
    latitude: 55.753936,
    longitude: 37.620793,
    category: 'памятник',
    images: [
      'https://upload.wikimedia.org/wikipedia/commons/thumb/e/e1/Red_Square_Moscow.jpg/640px-Red_Square_Moscow.jpg',
    ],
  },
  {
    id: 'kremlin',
    name: 'Московский Кремль',
    description:
      'Крепость в центре Москвы, резиденция Президента России. Символ российской государственности.',
    latitude: 55.751996,
    longitude: 37.617734,
    category: 'архитектура',
    images: [
      'https://upload.wikimedia.org/wikipedia/commons/thumb/0/04/Moscow_Kremlin_2013.jpg/640px-Moscow_Kremlin_2013.jpg',
    ],
  },
  {
    id: 'st-basils',
    name: 'Собор Василия Блаженного',
    description:
      'Один из самых узнаваемых храмов Москвы, памятник русской архитектуры XVI века.',
    latitude: 55.752521,
    longitude: 37.623089,
    category: 'храм',
    images: [
      'https://upload.wikimedia.org/wikipedia/commons/thumb/b/b7/St._Basil%27s_Cathedral%2C_Moscow.jpg/640px-St._Basil%27s_Cathedral%2C_Moscow.jpg',
    ],
  },
  {
    id: 'history-museum',
    name: 'Государственный исторический музей',
    description:
      'Крупнейший национальный исторический музей России, расположен на Красной площади.',
    latitude: 55.752212,
    longitude: 37.62152,
    category: 'музей',
    images: [
      'https://upload.wikimedia.org/wikipedia/commons/thumb/7/77/State_Historical_Museum_Moscow.jpg/640px-State_Historical_Museum_Moscow.jpg',
    ],
  },
  {
    id: 'bolshoi',
    name: 'Большой театр',
    description:
      'Один из крупнейших оперных и балетных театров мира, символ русской культуры.',
    latitude: 55.760446,
    longitude: 37.61805,
    category: 'театр',
    images: [
      'https://upload.wikimedia.org/wikipedia/commons/thumb/5/5b/Bolshoi_Theatre_Moscow.jpg/640px-Bolshoi_Theatre_Moscow.jpg',
    ],
  },
]

export function getCategoryColor(category: string): string {
  const colors: Record<string, string> = {
    памятник: '#dc2626',
    архитектура: '#2563eb',
    храм: '#7c3aed',
    музей: '#059669',
    театр: '#d97706',
  }
  return colors[category] || '#6b7280'
}
