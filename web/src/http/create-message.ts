interface GetMessageRequest {
  roomId: string
  message: string
}

export async function createMessage({ roomId, message }: GetMessageRequest) {
  const response = await fetch(
    `${import.meta.env.VITE_APP_API_URL}/rooms/${roomId}/messages`,
    {
      method: 'POST',
      body: JSON.stringify({ roomId, message }),
    },
  )

  const data: { id: string } = await response.json()

  return { messageId: data.id }
}
