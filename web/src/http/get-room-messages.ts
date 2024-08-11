interface GetRoomMessagesRequest {
  roomId: string
}

export interface GetRoomMessagesResponse {
  messages: {
    id: string
    text: string
    amountOfReaction: number
    answered: boolean
  }[]
}

export async function getRoomMessages({
  roomId,
}: GetRoomMessagesRequest): Promise<GetRoomMessagesResponse> {
  const response = await fetch(
    `${import.meta.env.VITE_APP_API_URL}/rooms/${roomId}/messages`,
  )

  const data: Array<{
    id: string
    roomId: string
    message: string
    reactionCount: number
    answered: boolean
  }> = await response.json()

  return {
    messages: data.map((item) => ({
      id: item.id,
      text: item.message,
      amountOfReaction: item.reactionCount,
      answered: item.answered,
    })),
  }
}
