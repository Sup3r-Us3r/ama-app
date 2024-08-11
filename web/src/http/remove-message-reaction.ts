interface RemoveMessageReactionRequest {
  roomId: string
  messageId: string
}

export async function removeMessageReaction({
  roomId,
  messageId,
}: RemoveMessageReactionRequest) {
  await fetch(
    `${import.meta.env.VITE_APP_API_URL}/rooms/${roomId}/messages/${messageId}/react`,
    {
      method: 'DELETE',
    },
  )
}
