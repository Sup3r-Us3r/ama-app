import { useSuspenseQuery } from '@tanstack/react-query'
import { useParams } from 'react-router-dom'

import { Message } from './message'
import { getRoomMessages } from '../http/get-room-messages'
import { useMessageWebSocket } from '../hooks/use-message-web-socket'

export const Messages = () => {
  const { roomId } = useParams()

  if (!roomId) {
    throw new Error('Message components must be used within room page')
  }

  const { data } = useSuspenseQuery({
    queryKey: ['@ama/messages', roomId],
    queryFn: () => getRoomMessages({ roomId }),
  })

  useMessageWebSocket({ roomId })

  const sortedMessages = data.messages.sort((a, b) => {
    return b.amountOfReaction - a.amountOfReaction
  })

  return (
    <ol className="list-decimal list-outside px-3 space-y-8">
      {sortedMessages.map((message) => (
        <Message
          key={message.id}
          id={message.id}
          text={message.text}
          amountOfReactions={message.amountOfReaction}
          answered={message.answered}
        />
      ))}
    </ol>
  )
}
